package action

import (
	"context"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
	"github.com/web3load/web3load/internal/plugin"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/txengine"
	"github.com/web3load/web3load/internal/wallet"
)

// buildExamplePlugin compiles examples/plugins/<name> into a temp binary
// so pluginAction can be exercised against a real, separate OS process.
func buildExamplePlugin(t *testing.T, name string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), name)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	srcDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", name))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, srcDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("building example plugin %s: %v", name, err)
	}
	return binPath
}

func TestPluginAction_ResultKind(t *testing.T) {
	binPath := buildExamplePlugin(t, "deadline")
	proc, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	act := &pluginAction{name: "deadline", process: proc}
	state := &VUState{Wallet: wallet.Wallet{Address: common.HexToAddress("0x1")}}
	step := scenario.Step{Action: "plugin:deadline", Args: []interface{}{300}}

	res, err := act.Execute(context.Background(), Deps{}, step, state)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatal("expected success")
	}
	out, ok := res.Value.(string)
	if !ok || out == "" {
		t.Fatalf("expected a non-empty string output value, got %#v", res.Value)
	}
}

func TestPluginAction_ErrorKind(t *testing.T) {
	binPath := buildExamplePlugin(t, "deadline")
	proc, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	act := &pluginAction{name: "deadline", process: proc}
	state := &VUState{Wallet: wallet.Wallet{Address: common.HexToAddress("0x1")}}
	step := scenario.Step{Action: "plugin:deadline", Args: []interface{}{"not-a-number"}}

	if _, err := act.Execute(context.Background(), Deps{}, step, state); err == nil {
		t.Fatal("expected an error from the plugin's error-kind response")
	}
}

// fakeSendAdapter succeeds at everything a native transfer needs, so
// pluginAction's "transaction" path can be exercised without a live chain.
type fakeSendAdapter struct {
	chain.Adapter
}

func (fakeSendAdapter) NonceAt(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}

func (fakeSendAdapter) SuggestFees(ctx context.Context) (*big.Int, *big.Int, error) {
	return big.NewInt(1), big.NewInt(1), nil
}

func (fakeSendAdapter) EstimateGas(ctx context.Context, req chain.TxRequest) (uint64, error) {
	return 21000, nil
}

func (fakeSendAdapter) SignTx(req chain.TxRequest, tipCap, feeCap *big.Int, privateKeyHex string) (*chain.SignedTx, error) {
	return &chain.SignedTx{Hash: common.HexToHash("0x1"), Nonce: req.Nonce, Raw: []byte{1}}, nil
}

func (fakeSendAdapter) SendRawTransaction(ctx context.Context, raw []byte) (common.Hash, error) {
	return common.HexToHash("0x2"), nil
}

func TestPluginAction_TransactionKind(t *testing.T) {
	binPath := buildExamplePlugin(t, "native_transfer")
	proc, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	adapter := fakeSendAdapter{}
	engine := txengine.New(adapter, wallet.NewNonceManager(adapter))
	deps := Deps{Adapter: adapter, Engine: engine}

	act := &pluginAction{name: "native_transfer", process: proc}
	state := &VUState{Wallet: wallet.Wallet{Address: common.HexToAddress("0x1"), PrivateKey: "0x01"}}
	step := scenario.Step{
		Action: "plugin:native_transfer",
		Args:   []interface{}{"0x000000000000000000000000000000000000dEaD", "1000000000000000"},
	}

	res, err := act.Execute(context.Background(), deps, step, state)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.TxHash == "" {
		t.Fatal("expected a tx hash from the transaction-kind response")
	}
}
