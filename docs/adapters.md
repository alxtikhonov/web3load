# Chain adapters

`internal/chain.Adapter` is the only boundary between the core engine and a
specific blockchain. The load engine, action registry, and transaction
engine only ever talk to this interface — they have no EVM-specific
knowledge. Adding a new chain means implementing this interface in a new
`internal/chain/<name>` package; nothing outside it should change.

```go
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
```

`internal/chain/evm` (v0.1) implements this on top of
`go-ethereum/ethclient`, building EIP-1559 (`DynamicFeeTx`) transactions.

## Known non-EVM gap

`common.Address`/`common.Hash` in `TxRequest`/`Receipt` are EVM-shaped
types reused directly from go-ethereum, which is a pragmatic v0.1 shortcut,
not a permanent design decision. A Solana (or other non-account-model, or
non-20-byte-address) adapter will need `TxRequest`/`Receipt` generalized —
e.g. `From`/`To` as opaque byte slices or a small `Address` wrapper type —
before it can implement this interface cleanly. Tracked as a v0.3
prerequisite, not something to work around inside an adapter.

## Why an interface instead of a plugin system in v0.1

Dynamic plugin loading (subprocess + gRPC, `hashicorp/go-plugin`-style) is
v0.3 scope — see the architecture doc. In v0.1, a new adapter is a Go
package behind this interface, registered at compile time; contributing one
means a PR, not shipping a separate binary. That's a deliberate sequencing
choice: get the interface boundary right first (so it doesn't leak
EVM-specific assumptions into `action`/`load`/`txengine`), then make it
dynamically loadable once there's a second real implementation to validate
the boundary against.
