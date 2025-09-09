// Package chain defines the boundary between the core engine and a specific
// blockchain. Everything outside this package and its subpackages (evm, ...)
// works only against the Adapter interface.
package chain

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// TxRequest describes an unsigned transaction before chain-specific
// encoding. See docs/adapters.md for the known EVM-shaped-types gap that a
// non-EVM adapter (e.g. Solana) will need to resolve.
type TxRequest struct {
	From     common.Address
	To       *common.Address // nil for contract creation; not used by any v0.1 action
	Value    *big.Int
	Data     []byte
	Nonce    uint64
	GasLimit uint64 // 0 tells the caller to estimate it
}

type SignedTx struct {
	Hash  common.Hash
	Nonce uint64
	Raw   []byte
}

type Receipt struct {
	TxHash      common.Hash
	Status      uint64 // 1 = success, 0 = reverted
	GasUsed     uint64
	BlockNumber uint64
}

// Adapter is implemented once per blockchain. New chains plug in by adding
// an implementation, never by changing this interface or its callers.
type Adapter interface {
	ChainID(ctx context.Context) (*big.Int, error)
	BalanceAt(ctx context.Context, addr common.Address) (*big.Int, error)
	NonceAt(ctx context.Context, addr common.Address) (uint64, error)
	EstimateGas(ctx context.Context, req TxRequest) (uint64, error)
	SuggestFees(ctx context.Context) (tipCap, feeCap *big.Int, err error)
	SignTx(req TxRequest, tipCap, feeCap *big.Int, privateKeyHex string) (*SignedTx, error)
	SendRawTransaction(ctx context.Context, raw []byte) (common.Hash, error)
	TransactionReceipt(ctx context.Context, hash common.Hash) (*Receipt, error)
	Call(ctx context.Context, req TxRequest) ([]byte, error)
}
