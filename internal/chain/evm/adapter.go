// Package evm implements chain.Adapter for Ethereum and any EVM-compatible
// RPC endpoint (Anvil, other L1/L2 nodes), using go-ethereum as the
// reference implementation of the wire format.
package evm

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/web3load/web3load/internal/chain"
)

type Adapter struct {
	client  *ethclient.Client
	chainID *big.Int
	signer  types.Signer
}

// Dial connects to an EVM RPC endpoint and verifies its chain id matches
// expectedChainID before returning. This is the primary guard against a
// scenario accidentally running against the wrong network; pass 0 to skip
// the check (used only by tooling that doesn't yet know the expected id).
func Dial(ctx context.Context, rpcURL string, expectedChainID int64) (*Adapter, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("evm: dial %s: %w", rpcURL, err)
	}
	actual, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("evm: fetch chain id from %s: %w", rpcURL, err)
	}
	if expectedChainID != 0 && actual.Int64() != expectedChainID {
		return nil, fmt.Errorf("evm: chain id mismatch: scenario expects %d, %s reports %s (refusing to run against the wrong chain)",
			expectedChainID, rpcURL, actual.String())
	}
	return &Adapter{
		client:  client,
		chainID: actual,
		signer:  types.LatestSignerForChainID(actual),
	}, nil
}

func (a *Adapter) ChainID(ctx context.Context) (*big.Int, error) {
	return a.chainID, nil
}

func (a *Adapter) BalanceAt(ctx context.Context, addr common.Address) (*big.Int, error) {
	bal, err := a.client.BalanceAt(ctx, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("evm: get balance of %s: %w", addr.Hex(), err)
	}
	return bal, nil
}

func (a *Adapter) NonceAt(ctx context.Context, addr common.Address) (uint64, error) {
	nonce, err := a.client.PendingNonceAt(ctx, addr)
	if err != nil {
		return 0, fmt.Errorf("evm: get nonce of %s: %w", addr.Hex(), err)
	}
	return nonce, nil
}

func (a *Adapter) EstimateGas(ctx context.Context, req chain.TxRequest) (uint64, error) {
	msg := ethereum.CallMsg{From: req.From, To: req.To, Value: req.Value, Data: req.Data}
	gas, err := a.client.EstimateGas(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("evm: estimate gas: %w", err)
	}
	return gas, nil
}

// SuggestFees returns an EIP-1559 tip cap and fee cap. The fee cap is the
// tip plus 2x the current base fee, giving headroom against a couple of
// blocks of base-fee increase before the transaction stops being includable.
func (a *Adapter) SuggestFees(ctx context.Context) (*big.Int, *big.Int, error) {
	tip, err := a.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("evm: suggest tip cap: %w", err)
	}
	head, err := a.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("evm: fetch latest header: %w", err)
	}
	if head.BaseFee == nil {
		// pre-EIP-1559 chain: fall back to legacy-equivalent fee cap = tip.
		return tip, new(big.Int).Set(tip), nil
	}
	feeCap := new(big.Int).Add(tip, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))
	return tip, feeCap, nil
}

func (a *Adapter) SignTx(req chain.TxRequest, tipCap, feeCap *big.Int, privateKeyHex string) (*chain.SignedTx, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("evm: parse private key: %w", err)
	}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   a.chainID,
		Nonce:     req.Nonce,
		GasTipCap: tipCap,
		GasFeeCap: feeCap,
		Gas:       req.GasLimit,
		To:        req.To,
		Value:     req.Value,
		Data:      req.Data,
	})
	signed, err := types.SignTx(tx, a.signer, key)
	if err != nil {
		return nil, fmt.Errorf("evm: sign tx: %w", err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("evm: encode signed tx: %w", err)
	}
	return &chain.SignedTx{Hash: signed.Hash(), Nonce: req.Nonce, Raw: raw}, nil
}

func (a *Adapter) SendRawTransaction(ctx context.Context, raw []byte) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, fmt.Errorf("evm: decode raw tx: %w", err)
	}
	if err := a.client.SendTransaction(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

// TransactionReceipt returns ethereum.NotFound (wrapped) while the
// transaction is still pending; callers poll on that specific condition.
func (a *Adapter) TransactionReceipt(ctx context.Context, hash common.Hash) (*chain.Receipt, error) {
	r, err := a.client.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &chain.Receipt{
		TxHash:      r.TxHash,
		Status:      r.Status,
		GasUsed:     r.GasUsed,
		BlockNumber: r.BlockNumber.Uint64(),
	}, nil
}

func (a *Adapter) Call(ctx context.Context, req chain.TxRequest) ([]byte, error) {
	msg := ethereum.CallMsg{From: req.From, To: req.To, Data: req.Data}
	out, err := a.client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("evm: eth_call: %w", err)
	}
	return out, nil
}
