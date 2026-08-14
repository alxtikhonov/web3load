// Package plugin implements Web3Load's plugin transport: a subprocess
// speaking line-delimited JSON on its stdin/stdout. This is deliberately
// not the gRPC/hashicorp-go-plugin design sketched in the architecture
// doc — see docs/plugins.md for why — but it keeps the promise that
// actually mattered: a plugin can be written in any language that can
// read a line from stdin and write a line to stdout, no Web3Load SDK or
// code generation required to build one.
package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Request is one step invocation, sent to the plugin as a single line of
// JSON on stdin. Wallet is an address only — plugins never receive key
// material; signing and submission stay inside the trusted host process.
type Request struct {
	Method   string                 `json:"method"`
	Contract string                 `json:"contract,omitempty"`
	Args     []interface{}          `json:"args,omitempty"`
	Vars     map[string]string      `json:"vars,omitempty"`
	Wallet   string                 `json:"wallet"`
	Saved    map[string]interface{} `json:"saved,omitempty"`
}

// Response is the plugin's answer, one line of JSON on stdout. Kind
// selects which shape applies:
//
//   - "transaction": the plugin computed what to send; the host builds,
//     signs, and submits it via the normal transaction engine.
//   - "result": a non-transactional outcome (e.g. a computed value to
//     save via save_as), with no chain interaction from this step.
//   - "error": the plugin failed; Error explains why.
type Response struct {
	Kind string `json:"kind"`

	To                  string `json:"to,omitempty"`
	Value               string `json:"value,omitempty"` // decimal wei string
	Data                string `json:"data,omitempty"`  // 0x-hex, may be empty
	GasLimit            uint64 `json:"gas_limit,omitempty"`
	WaitForConfirmation bool   `json:"wait_for_confirmation,omitempty"`

	Success bool   `json:"success,omitempty"`
	Output  string `json:"output,omitempty"`

	Error string `json:"error,omitempty"`
}

// Process manages one running plugin subprocess. Calls are serialized
// through a mutex — v0.3 runs one subprocess per plugin, shared by every
// VU that uses it, not a pool per VU; see docs/plugins.md for the
// throughput implication and why that's an acceptable v0.3 limit.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	mu     sync.Mutex
}

// Start launches path as a subprocess. The plugin's stderr is connected to
// this process's stderr, so a plugin's own logs or panics are visible to
// the operator instead of vanishing silently.
func Start(path string, args ...string) (*Process, error) {
	cmd := exec.Command(path, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin: stdin pipe for %s: %w", path, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin: stdout pipe for %s: %w", path, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin: start %s: %w", path, err)
	}

	return &Process{cmd: cmd, stdin: stdin, reader: bufio.NewReader(stdout)}, nil
}

// Call sends req and blocks for the matching response. There is no
// request id or pipelining in v0.3 — Call holds the mutex for the whole
// round trip, so concurrent VUs sharing a plugin queue up one at a time.
func (p *Process) Call(req Request) (Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("plugin: encode request: %w", err)
	}
	data = append(data, '\n')
	if _, err := p.stdin.Write(data); err != nil {
		return Response{}, fmt.Errorf("plugin: write request: %w", err)
	}

	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		return Response{}, fmt.Errorf("plugin: read response: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("plugin: decode response %q: %w", line, err)
	}
	return resp, nil
}

// Close signals the plugin to exit — closing its stdin gives it an EOF,
// the same shutdown signal any well-behaved read-loop plugin should
// already handle — and waits for it.
func (p *Process) Close() error {
	if err := p.stdin.Close(); err != nil {
		return fmt.Errorf("plugin: close stdin: %w", err)
	}
	return p.cmd.Wait()
}
