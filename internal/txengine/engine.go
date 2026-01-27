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
