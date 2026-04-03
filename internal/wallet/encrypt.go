package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/scrypt"
)

// scrypt parameters follow the widely-used geth/ethers keystore defaults:
// expensive enough to make offline brute-forcing costly, cheap enough to
// decrypt in well under a second on ordinary hardware.
const (
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
	keyLen  = 32
)

// encryptedFile is the on-disk envelope for an encrypted keystore: enough
// of the KDF/cipher parameters to decrypt are stored alongside the
// ciphertext, standard practice for password-based encryption at rest.
type encryptedFile struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	N          int    `json:"n"`
	R          int    `json:"r"`
	P          int    `json:"p"`
	Cipher     string `json:"cipher"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// SaveEncrypted writes ks to path as AES-256-GCM ciphertext, keyed by
// scrypt(password, random salt). Prefer this over Save whenever the
// keystore might touch a real network — see docs/security.md.
func (ks *Keystore) SaveEncrypted(path, password string) error {
	if password == "" {
		return fmt.Errorf("wallet: encryption password must not be empty")
	}
	plaintext, err := json.Marshal(ks)
	if err != nil {
		return fmt.Errorf("wallet: marshal keystore: %w", err)
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("wallet: generate salt: %w", err)
	}
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return fmt.Errorf("wallet: derive key: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("wallet: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	ef := encryptedFile{
		Version:    1,
		KDF:        "scrypt",
		Salt:       hex.EncodeToString(salt),
		N:          scryptN,
		R:          scryptR,
		P:          scryptP,
		Cipher:     "aes-256-gcm",
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(ciphertext),
	}
	data, err := json.MarshalIndent(ef, "", "  ")
	if err != nil {
		return fmt.Errorf("wallet: marshal encrypted keystore: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("wallet: write encrypted keystore %s: %w", path, err)
	}
	return nil
}

// LoadEncrypted reads and decrypts a keystore written by SaveEncrypted. A
// wrong password and a corrupted file are indistinguishable to AES-GCM by
// design (both fail authentication) and are reported the same way.
func LoadEncrypted(path, password string) (*Keystore, error) {
	if password == "" {
		return nil, fmt.Errorf("wallet: %s is encrypted, a password is required (--password or WEB3LOAD_KEYSTORE_PASSWORD)", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wallet: read keystore %s: %w", path, err)
	}
	var ef encryptedFile
	if err := json.Unmarshal(data, &ef); err != nil {
		return nil, fmt.Errorf("wallet: parse encrypted keystore %s: %w", path, err)
	}

	salt, err := hex.DecodeString(ef.Salt)
	if err != nil {
		return nil, fmt.Errorf("wallet: %s: invalid salt: %w", path, err)
	}
	nonce, err := hex.DecodeString(ef.Nonce)
	if err != nil {
		return nil, fmt.Errorf("wallet: %s: invalid nonce: %w", path, err)
	}
	ciphertext, err := hex.DecodeString(ef.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("wallet: %s: invalid ciphertext: %w", path, err)
	}

	key, err := scrypt.Key([]byte(password), salt, ef.N, ef.R, ef.P, keyLen)
	if err != nil {
		return nil, fmt.Errorf("wallet: derive key: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("wallet: decrypt %s: wrong password or corrupted file", path)
	}

	var ks Keystore
	if err := json.Unmarshal(plaintext, &ks); err != nil {
		return nil, fmt.Errorf("wallet: parse decrypted keystore: %w", err)
	}
	return &ks, nil
}

// IsEncrypted sniffs whether path is one of our encrypted envelopes (has a
// "kdf" field) versus a plaintext {"wallets": [...]} keystore, without
// needing the password.
func IsEncrypted(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("wallet: read %s: %w", path, err)
	}
	var probe struct {
		KDF string `json:"kdf"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false, fmt.Errorf("wallet: parse %s: %w", path, err)
	}
	return probe.KDF != "", nil
}

// LoadAny loads a keystore file regardless of whether it's plaintext or
// encrypted, detecting the format automatically. password is ignored (and
// may be empty) for plaintext files.
func LoadAny(path, password string) (*Keystore, error) {
	encrypted, err := IsEncrypted(path)
	if err != nil {
		return nil, err
	}
	if encrypted {
		return LoadEncrypted(path, password)
	}
	return Load(path)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("wallet: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("wallet: create gcm: %w", err)
	}
	return gcm, nil
}
