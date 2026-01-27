package txengine

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
	"github.com/web3load/web3load/internal/wallet"
)

// resyncTrackingAdapter always fails SendRawTransaction with a nonce error,
// so Send's resync-on-nonce-error path can be verified without a live chain.
type resyncTrackingAdapter struct {
	chain.Adapter
	nonceAtCalls int
}

func (a *resyncTrackingAdapter) NonceAt(ctx context.Context, addr common.Address) (uint64, error) {
	a.nonceAtCalls++
	return 0, nil
}

func (a *resyncTrackingAdapter) SuggestFees(ctx context.Context) (*big.Int, *big.Int, error) {
	return big.NewInt(1), big.NewInt(1), nil
}

func (a *resyncTrackingAdapter) EstimateGas(ctx context.Context, req chain.TxRequest) (uint64, error) {
	return 21000, nil
}

func (a *resyncTrackingAdapter) SignTx(req chain.TxRequest, tipCap, feeCap *big.Int, privateKeyHex string) (*chain.SignedTx, error) {
	return &chain.SignedTx{Hash: common.HexToHash("0x1"), Nonce: req.Nonce, Raw: []byte{1, 2, 3}}, nil
}

func (a *resyncTrackingAdapter) SendRawTransaction(ctx context.Context, raw []byte) (common.Hash, error) {
	return common.Hash{}, errors.New("nonce too low")
}

func TestSend_ResyncsNonceOnNonceError(t *testing.T) {
	adapter := &resyncTrackingAdapter{}
	nonces := wallet.NewNonceManager(adapter)
	engine := New(adapter, nonces)

	w := wallet.Wallet{Address: common.HexToAddress("0xabc"), PrivateKey: "0x01"}
	to := common.HexToAddress("0xdef")

	_, err := engine.Send(context.Background(), w, &to, big.NewInt(0), nil, 21000, false)
	if err == nil {
		t.Fatal("expected an error from SendRawTransaction")
	}
	// One NonceAt call to seed the initial nonce, one more triggered by
	// Resync after the "nonce too low" error.
	if adapter.nonceAtCalls != 2 {
		t.Fatalf("expected NonceAt to be called twice (seed + resync), got %d", adapter.nonceAtCalls)
	}
}

func TestIsNonceError(t *testing.T) {
	cases := map[string]bool{
		"nonce too low":      true,
		"nonce too high":     true,
		"insufficient funds": false,
	}
	for msg, want := range cases {
		if got := isNonceError(errors.New(msg)); got != want {
			t.Errorf("isNonceError(%q) = %v, want %v", msg, got, want)
		}
	}
}
