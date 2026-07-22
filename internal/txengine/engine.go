// Package txengine builds, signs, submits, and (optionally) confirms
// transactions, and is the one place that distinguishes submission latency
// (the RPC accepted the transaction) from confirmation latency (it was
// mined) — a distinction generic HTTP load tools have no concept of.
package txengine

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
	"github.com/web3load/web3load/internal/wallet"
)

type Outcome struct {
	Hash           common.Hash
	Waited         bool // true if confirmation was polled for (see Send)
	Success        bool
	Reverted       bool
	GasUsed        uint64
	SubmitLatency  time.Duration
	ConfirmLatency time.Duration
}

type Engine struct {
	Adapter        chain.Adapter
	Nonces         *wallet.NonceManager
	ConfirmTimeout time.Duration
	PollInterval   time.Duration

	// MaxGasPriceGwei, if > 0, clamps the fee cap (and tip cap, so it never
	// exceeds the clamped fee cap) suggested by the adapter before signing.
	// Corresponds to scenario safety.max_gas_price_gwei — see
	// docs/security.md.
	MaxGasPriceGwei float64

	// DryRun builds and signs every transaction exactly as a real run
	// would (so gas estimation and signing are still exercised) but stops
	// short of SendRawTransaction. The allocated nonce is released
	// immediately since nothing was actually consumed on-chain.
	DryRun bool
}

func New(adapter chain.Adapter, nonces *wallet.NonceManager) *Engine {
	return &Engine{
		Adapter:        adapter,
		Nonces:         nonces,
		ConfirmTimeout: 60 * time.Second,
		PollInterval:   500 * time.Millisecond,
	}
}

// Send builds, signs, and submits a transaction from `from`. If
// waitForConfirmation is true it then polls for the receipt, classifying
// the result as success or revert and recording confirmation latency
// separately from submission latency.
func (e *Engine) Send(ctx context.Context, from wallet.Wallet, to *common.Address, value *big.Int, data []byte, gasLimit uint64, waitForConfirmation bool) (Outcome, error) {
	fromAddr := from.Address
	nonce, err := e.Nonces.Next(ctx, fromAddr)
	if err != nil {
		return Outcome{}, fmt.Errorf("txengine: allocate nonce: %w", err)
	}

	tipCap, feeCap, err := e.Adapter.SuggestFees(ctx)
	if err != nil {
		e.Nonces.Release(fromAddr, nonce)
		return Outcome{}, fmt.Errorf("txengine: suggest fees: %w", err)
	}
	if e.MaxGasPriceGwei > 0 {
		tipCap, feeCap = clampGasPrice(tipCap, feeCap, e.MaxGasPriceGwei, fromAddr)
	}

	req := chain.TxRequest{From: fromAddr, To: to, Value: value, Data: data, Nonce: nonce, GasLimit: gasLimit}
	if req.GasLimit == 0 {
		estimated, err := e.Adapter.EstimateGas(ctx, req)
		if err != nil {
			e.Nonces.Release(fromAddr, nonce)
			return Outcome{}, fmt.Errorf("txengine: estimate gas: %w", err)
		}
		req.GasLimit = estimated + estimated/5 // 20% headroom over the estimate
	}

	signed, err := e.Adapter.SignTx(req, tipCap, feeCap, from.PrivateKey)
	if err != nil {
		e.Nonces.Release(fromAddr, nonce)
		return Outcome{}, fmt.Errorf("txengine: sign: %w", err)
	}

	if e.DryRun {
		e.Nonces.Release(fromAddr, nonce)
		return Outcome{Success: true}, nil
	}

	submitStart := time.Now()
	hash, err := e.Adapter.SendRawTransaction(ctx, signed.Raw)
	if err != nil {
		e.Nonces.Release(fromAddr, nonce)
		if isNonceError(err) {
			// Local nonce tracking has drifted from the chain's — most
			// commonly after an external transaction from the same wallet,
			// or a dropped/replaced transaction. Resync so the *next*
			// iteration for this wallet doesn't keep failing the same way.
			if rerr := e.Nonces.Resync(ctx, fromAddr); rerr != nil {
				slog.Warn("txengine: nonce resync failed", "wallet", fromAddr.Hex(), "error", rerr)
			} else {
				slog.Info("txengine: nonce resynced after mismatch", "wallet", fromAddr.Hex())
			}
		}
		return Outcome{}, fmt.Errorf("txengine: submit: %w", err)
	}
	out := Outcome{Hash: hash, SubmitLatency: time.Since(submitStart)}

	if !waitForConfirmation {
		return out, nil
	}
	return e.waitForConfirmation(ctx, out)
}

func isNonceError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "nonce too low") || strings.Contains(msg, "nonce too high")
}

// clampGasPrice caps feeCap at maxGasPriceGwei, and tipCap at whatever
// feeCap ends up being — a tip above the fee cap is an invalid EIP-1559
// transaction, so clamping only the fee cap and leaving tip alone could
// turn a "too expensive" scenario into a "the RPC rejects this tx outright"
// one, which is a worse failure mode than the cap it was meant to enforce.
func clampGasPrice(tipCap, feeCap *big.Int, maxGasPriceGwei float64, wallet common.Address) (*big.Int, *big.Int) {
	maxWei := gweiToWei(maxGasPriceGwei)
	if feeCap.Cmp(maxWei) <= 0 {
		return tipCap, feeCap
	}
	slog.Warn("txengine: clamping gas price to safety.max_gas_price_gwei",
		"wallet", wallet.Hex(), "suggested_fee_cap_wei", feeCap.String(), "max_gwei", maxGasPriceGwei)
	feeCap = maxWei
	if tipCap.Cmp(feeCap) > 0 {
		tipCap = new(big.Int).Set(feeCap)
	}
	return tipCap, feeCap
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(1e9)).Int(nil)
	return wei
}

func (e *Engine) waitForConfirmation(ctx context.Context, out Outcome) (Outcome, error) {
	out.Waited = true
	confirmStart := time.Now()
	deadline := confirmStart.Add(e.ConfirmTimeout)

	for {
		receipt, err := e.Adapter.TransactionReceipt(ctx, out.Hash)
		if err == nil {
			out.ConfirmLatency = time.Since(confirmStart)
			out.GasUsed = receipt.GasUsed
			out.Success = receipt.Status == 1
			out.Reverted = receipt.Status == 0
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("txengine: tx %s not confirmed within %s", out.Hash.Hex(), e.ConfirmTimeout)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(e.PollInterval):
		}
	}
}
