// Package wallet manages virtual-user identities: key generation, the
// on-disk keystore, per-address nonce sequencing under concurrency, and
// bootstrapping wallets with funds before a scenario can run.
package wallet

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Wallet struct {
	Address    common.Address `json:"address"`
	PrivateKey string         `json:"private_key"`
}

// String redacts the private key so an accidental fmt.Println(wallet) or a
// %+v in a log line never leaks it. This covers %v, %s, and %+v — anything
// that respects fmt.Stringer — but not %#v, which callers must not use on
// wallet values.
func (w Wallet) String() string {
	return fmt.Sprintf("Wallet{%s}", w.Address.Hex())
}

func (w Wallet) ECDSA() (*ecdsa.PrivateKey, error) {
	if len(w.PrivateKey) < 2 {
		return nil, fmt.Errorf("wallet: empty private key for %s", w.Address.Hex())
	}
	hexKey := w.PrivateKey
	if hexKey[0:2] == "0x" {
		hexKey = hexKey[2:]
	}
	return crypto.HexToECDSA(hexKey)
}

func fromECDSA(key *ecdsa.PrivateKey) Wallet {
	return Wallet{
		Address:    crypto.PubkeyToAddress(key.PublicKey),
		PrivateKey: "0x" + fmt.Sprintf("%x", crypto.FromECDSA(key)),
	}
}
