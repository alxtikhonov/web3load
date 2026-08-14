package plugin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/web3load/web3load/internal/plugin"
)

// buildExamplePlugin compiles examples/plugins/deadline into a temp binary
// so Process can be exercised against a real, separate OS process — the
// actual mechanism plugins use, not a mock of it.
func buildExamplePlugin(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "deadline")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	srcDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", "deadline"))
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "build", "-o", binPath, srcDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("building example plugin at %s: %v", srcDir, err)
	}
	return binPath
}

func TestProcess_CallsExamplePlugin(t *testing.T) {
	binPath := buildExamplePlugin(t)

	p, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	resp, err := p.Call(plugin.Request{Method: "deadline", Args: []interface{}{300}, Wallet: "0xabc"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != "result" || !resp.Success {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Output == "" {
		t.Fatal("expected a non-empty output timestamp")
	}
}

func TestProcess_MultipleSequentialCalls(t *testing.T) {
	binPath := buildExamplePlugin(t)
	p, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	for i := 0; i < 3; i++ {
		resp, err := p.Call(plugin.Request{Method: "deadline", Args: []interface{}{float64(i)}, Wallet: "0xabc"})
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if resp.Kind != "result" {
			t.Fatalf("call %d: unexpected kind %q", i, resp.Kind)
		}
	}
}

func TestProcess_ErrorResponseOnBadArgs(t *testing.T) {
	binPath := buildExamplePlugin(t)
	p, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	resp, err := p.Call(plugin.Request{Method: "deadline", Args: []interface{}{"not a number"}, Wallet: "0xabc"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != "error" {
		t.Fatalf("expected an error response for a non-numeric arg, got %+v", resp)
	}
}

func TestProcess_CloseWaitsForExit(t *testing.T) {
	binPath := buildExamplePlugin(t)
	p, err := plugin.Start(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(plugin.Request{Method: "deadline", Args: []interface{}{1}, Wallet: "0xabc"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("expected the plugin to exit cleanly on stdin close, got: %v", err)
	}
}
