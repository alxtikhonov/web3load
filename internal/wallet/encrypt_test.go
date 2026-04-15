package wallet

import (
	"path/filepath"
	"testing"
)

func TestEncryptedKeystore_RoundTrip(t *testing.T) {
	ks, err := Generate(3)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "wallets.enc.json")

	if err := ks.SaveEncrypted(path, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadEncrypted(path, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Wallets) != len(ks.Wallets) {
		t.Fatalf("expected %d wallets, got %d", len(ks.Wallets), len(loaded.Wallets))
	}
	for i := range ks.Wallets {
		if loaded.Wallets[i].Address != ks.Wallets[i].Address {
			t.Errorf("wallet %d: address mismatch", i)
		}
		if loaded.Wallets[i].PrivateKey != ks.Wallets[i].PrivateKey {
			t.Errorf("wallet %d: private key mismatch after round trip", i)
		}
	}
}

func TestEncryptedKeystore_WrongPasswordFails(t *testing.T) {
	ks, err := Generate(1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "wallets.enc.json")
	if err := ks.SaveEncrypted(path, "right password"); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadEncrypted(path, "wrong password"); err == nil {
		t.Fatal("expected decryption with the wrong password to fail")
	}
}

func TestEncryptedKeystore_EmptyPasswordRejected(t *testing.T) {
	ks, err := Generate(1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "wallets.enc.json")
	if err := ks.SaveEncrypted(path, ""); err == nil {
		t.Fatal("expected an empty encryption password to be rejected")
	}
}

func TestIsEncrypted_DistinguishesFormats(t *testing.T) {
	dir := t.TempDir()

	plainPath := filepath.Join(dir, "plain.json")
	ks, err := Generate(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.Save(plainPath); err != nil {
		t.Fatal(err)
	}
	if encrypted, err := IsEncrypted(plainPath); err != nil || encrypted {
		t.Fatalf("expected plaintext keystore to report IsEncrypted=false, got %v, err=%v", encrypted, err)
	}

	encPath := filepath.Join(dir, "enc.json")
	if err := ks.SaveEncrypted(encPath, "password123"); err != nil {
		t.Fatal(err)
	}
	if encrypted, err := IsEncrypted(encPath); err != nil || !encrypted {
		t.Fatalf("expected encrypted keystore to report IsEncrypted=true, got %v, err=%v", encrypted, err)
	}
}

func TestLoadAny_DispatchesByFormat(t *testing.T) {
	dir := t.TempDir()
	ks, err := Generate(2)
	if err != nil {
		t.Fatal(err)
	}

	plainPath := filepath.Join(dir, "plain.json")
	if err := ks.Save(plainPath); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAny(plainPath, ""); err != nil {
		t.Fatalf("LoadAny on a plaintext keystore should not require a password: %v", err)
	}

	encPath := filepath.Join(dir, "enc.json")
	if err := ks.SaveEncrypted(encPath, "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAny(encPath, "s3cr3t"); err != nil {
		t.Fatalf("LoadAny on an encrypted keystore with the right password: %v", err)
	}
	if _, err := LoadAny(encPath, ""); err == nil {
		t.Fatal("expected LoadAny on an encrypted keystore without a password to fail")
	}
}
