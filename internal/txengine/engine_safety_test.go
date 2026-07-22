package txengine

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
	"github.com/web3load/web3load/internal/wallet"
)

// recordingAdapter succeeds at everything and records the fee/tip cap it
// was asked to sign with, plus whether SendRawTransaction was ever called —
// enough to verify gas-price clamping and dry-run's "never broadcasts"
// guarantee without a live chain.
type recordingAdapter struct {
	chain.Adapter
	suggestedTip, suggestedFee *big.Int
	signedTip, signedFee       *big.Int
	sendRawTransactionCalled   bool
}

func (a *recordingAdapter) NonceAt(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}

func (a *recordingAdapter) SuggestFees(ctx context.Context) (*big.Int, *big.Int, error) {
	return a.suggestedTip, a.suggestedFee, nil
}

func (a *recordingAdapter) EstimateGas(ctx context.Context, req chain.TxRequest) (uint64, error) {
	return 21000, nil
}

func (a *recordingAdapter) SignTx(req chain.TxRequest, tipCap, feeCap *big.Int, privateKeyHex string) (*chain.SignedTx, error) {
	a.signedTip, a.signedFee = tipCap, feeCap
	return &chain.SignedTx{Hash: common.HexToHash("0x1"), Nonce: req.Nonce, Raw: []byte{1, 2, 3}}, nil
}

func (a *recordingAdapter) SendRawTransaction(ctx context.Context, raw []byte) (common.Hash, error) {
	a.sendRawTransactionCalled = true
	return common.HexToHash("0x2"), nil
}

func TestSend_ClampsGasPriceToSafetyLimit(t *testing.T) {
	adapter := &recordingAdapter{suggestedTip: gweiToWei(5), suggestedFee: gweiToWei(100)}
	nonces := wallet.NewNonceManager(adapter)
	engine := New(adapter, nonces)
	engine.MaxGasPriceGwei = 20

	w := wallet.Wallet{Address: common.HexToAddress("0xabc"), PrivateKey: "0x01"}
	to := common.HexToAddress("0xdef")

	if _, err := engine.Send(context.Background(), w, &to, big.NewInt(0), nil, 21000, false); err != nil {
		t.Fatal(err)
	}

	want := gweiToWei(20)
	if adapter.signedFee.Cmp(want) != 0 {
		t.Errorf("expected fee cap clamped to 20 gwei (%s wei), got %s", want, adapter.signedFee)
	}
	if adapter.signedTip.Cmp(adapter.signedFee) > 0 {
		t.Errorf("expected tip cap <= clamped fee cap, got tip=%s fee=%s", adapter.signedTip, adapter.signedFee)
	}
}

func TestSend_DoesNotClampBelowSafetyLimit(t *testing.T) {
	adapter := &recordingAdapter{suggestedTip: gweiToWei(1), suggestedFee: gweiToWei(10)}
	nonces := wallet.NewNonceManager(adapter)
	engine := New(adapter, nonces)
	engine.MaxGasPriceGwei = 20

	w := wallet.Wallet{Address: common.HexToAddress("0xabc"), PrivateKey: "0x01"}
	to := common.HexToAddress("0xdef")

	if _, err := engine.Send(context.Background(), w, &to, big.NewInt(0), nil, 21000, false); err != nil {
		t.Fatal(err)
	}

	if adapter.signedFee.Cmp(gweiToWei(10)) != 0 {
		t.Errorf("expected fee cap left untouched at 10 gwei, got %s", adapter.signedFee)
	}
}

func TestSend_DryRunNeverBroadcasts(t *testing.T) {
	adapter := &recordingAdapter{suggestedTip: gweiToWei(1), suggestedFee: gweiToWei(5)}
	nonces := wallet.NewNonceManager(adapter)
	engine := New(adapter, nonces)
	engine.DryRun = true

	w := wallet.Wallet{Address: common.HexToAddress("0xabc"), PrivateKey: "0x01"}
	to := common.HexToAddress("0xdef")

	out, err := engine.Send(context.Background(), w, &to, big.NewInt(0), nil, 21000, true)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Error("expected a dry-run Send to report Success")
	}
	if adapter.sendRawTransactionCalled {
		t.Fatal("dry-run must never call SendRawTransaction")
	}
}

func TestSend_DryRunReleasesNonce(t *testing.T) {
	adapter := &recordingAdapter{suggestedTip: gweiToWei(1), suggestedFee: gweiToWei(5)}
	nonces := wallet.NewNonceManager(adapter)
	engine := New(adapter, nonces)
	engine.DryRun = true

	w := wallet.Wallet{Address: common.HexToAddress("0xabc"), PrivateKey: "0x01"}
	to := common.HexToAddress("0xdef")

	if _, err := engine.Send(context.Background(), w, &to, big.NewInt(0), nil, 21000, false); err != nil {
		t.Fatal(err)
	}

	// A second allocation from the same wallet should get the same nonce
	// back, proving the dry-run released it instead of consuming it.
	nonce, err := nonces.Next(context.Background(), w.Address)
	if err != nil {
		t.Fatal(err)
	}
	if nonce != 0 {
		t.Fatalf("expected the dry-run's nonce to be released back to 0, got %d", nonce)
	}
}

func TestGweiToWei(t *testing.T) {
	got := gweiToWei(1.5)
	want := big.NewInt(1_500_000_000)
	if got.Cmp(want) != 0 {
		t.Errorf("gweiToWei(1.5) = %s, want %s", got, want)
	}
}
