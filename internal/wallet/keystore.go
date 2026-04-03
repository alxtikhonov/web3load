package wallet

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

type Keystore struct {
	Wallets []Wallet `json:"wallets"`
}

// Generate creates count distinct wallets. Addresses are checked for
// collisions as a sanity assertion, not because secp256k1 key generation is
// expected to collide in practice.
func Generate(count int) (*Keystore, error) {
	if count <= 0 {
		return nil, fmt.Errorf("wallet: count must be positive, got %d", count)
	}
	ks := &Keystore{Wallets: make([]Wallet, 0, count)}
	seen := make(map[string]struct{}, count)
	for i := 0; i < count; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("wallet: generate key %d: %w", i, err)
		}
		w := fromECDSA(key)
		if _, dup := seen[w.Address.Hex()]; dup {
			return nil, fmt.Errorf("wallet: duplicate address generated (should never happen): %s", w.Address.Hex())
		}
		seen[w.Address.Hex()] = struct{}{}
		ks.Wallets = append(ks.Wallets, w)
	}
	return ks, nil
}

// Save writes the keystore as plaintext JSON with owner-only permissions.
// Prefer SaveEncrypted whenever the keystore might touch a real network —
// see docs/security.md.
func (ks *Keystore) Save(path string) error {
	data, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return fmt.Errorf("wallet: marshal keystore: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("wallet: write keystore %s: %w", path, err)
	}
	return nil
}

func Load(path string) (*Keystore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wallet: read keystore %s: %w", path, err)
	}
	var ks Keystore
	if err := json.Unmarshal(data, &ks); err != nil {
		return nil, fmt.Errorf("wallet: parse keystore %s: %w", path, err)
	}
	return &ks, nil
}

func (ks *Keystore) String() string {
	return fmt.Sprintf("<keystore: %d wallets, private keys redacted>", len(ks.Wallets))
}
