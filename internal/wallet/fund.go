package wallet

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/web3load/web3load/internal/chain"
)

type FundResult struct {
	Address common.Address
	TxHash  common.Hash
	Err     error
}

// Fund sends amountWei of native currency to each wallet from a single
// funder key, sequentially — the funder is one account and must not race
// its own nonce — waiting for each transfer to be mined before the next.
func Fund(ctx context.Context, adapter chain.Adapter, funderPrivateKeyHex string, wallets []Wallet, amountWei *big.Int, confirmTimeout time.Duration) []FundResult {
	funder := Wallet{PrivateKey: funderPrivateKeyHex}
	key, err := funder.ECDSA()
	if err != nil {
		results := make([]FundResult, len(wallets))
		for i, w := range wallets {
			results[i] = FundResult{Address: w.Address, Err: fmt.Errorf("wallet: invalid funder key: %w", err)}
		}
		return results
	}
	from := crypto.PubkeyToAddress(key.PublicKey)

	nonces := NewNonceManager(adapter)
	results := make([]FundResult, 0, len(wallets))

	for _, w := range wallets {
		res := FundResult{Address: w.Address}

		nonce, err := nonces.Next(ctx, from)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		tipCap, feeCap, err := adapter.SuggestFees(ctx)
		if err != nil {
			nonces.Release(from, nonce)
			res.Err = fmt.Errorf("wallet: suggest fees: %w", err)
			results = append(results, res)
			continue
		}
		to := w.Address
		signed, err := adapter.SignTx(chain.TxRequest{
			From:     from,
			To:       &to,
			Value:    amountWei,
			Nonce:    nonce,
			GasLimit: 21000,
		}, tipCap, feeCap, funderPrivateKeyHex)
		if err != nil {
			nonces.Release(from, nonce)
			res.Err = fmt.Errorf("wallet: sign funding tx: %w", err)
			results = append(results, res)
			continue
		}
		hash, err := adapter.SendRawTransaction(ctx, signed.Raw)
		if err != nil {
			nonces.Release(from, nonce)
			res.Err = fmt.Errorf("wallet: send funding tx: %w", err)
			results = append(results, res)
			continue
		}
		res.TxHash = hash
		res.Err = waitForReceipt(ctx, adapter, hash, confirmTimeout)
		results = append(results, res)
	}
	return results
}

func waitForReceipt(ctx context.Context, adapter chain.Adapter, hash common.Hash, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		receipt, err := adapter.TransactionReceipt(ctx, hash)
		if err == nil {
			if receipt.Status == 0 {
				return fmt.Errorf("wallet: funding tx %s reverted", hash.Hex())
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wallet: funding tx %s not confirmed within %s", hash.Hex(), timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
