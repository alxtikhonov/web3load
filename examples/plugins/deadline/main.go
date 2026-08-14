// Command deadline is an example Web3Load plugin: given `args: [<seconds
// from now>]`, it returns the computed unix timestamp as its output value
// (saveable via save_as). It demonstrates the "result" half of the plugin
// protocol — a non-transactional step that never touches the chain — and
// exists partly to fill the one documented DSL gap (relative-time
// expressions like `${now + 300}` aren't supported in scenario YAML
// itself, see docs/dsl-reference.md) using the extension mechanism instead
// of a core change.
//
// This file deliberately imports nothing from Web3Load: a plugin is any
// program that reads one line of JSON per request from stdin and writes
// one line of JSON back to stdout — see docs/plugins.md.
//
// Build: go build -o deadline ./examples/plugins/deadline
// Use:   web3load run scenario.yaml --plugin deadline=./deadline
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type request struct {
	Method string        `json:"method"`
	Args   []interface{} `json:"args"`
}

type response struct {
	Kind    string `json:"kind"`
	Success bool   `json:"success,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			handle(line, writer)
			writer.Flush()
		}
		if readErr != nil {
			return
		}
	}
}

func handle(line []byte, w *bufio.Writer) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		writeResp(w, response{Kind: "error", Error: err.Error()})
		return
	}

	seconds, ok := numberArg(req.Args, 0)
	if !ok {
		writeResp(w, response{Kind: "error", Error: "deadline: expected args[0] to be a number of seconds from now"})
		return
	}

	deadline := time.Now().Add(time.Duration(seconds) * time.Second).Unix()
	writeResp(w, response{Kind: "result", Success: true, Output: fmt.Sprintf("%d", deadline)})
}

func numberArg(args []interface{}, i int) (float64, bool) {
	if i >= len(args) {
		return 0, false
	}
	switch v := args[i].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func writeResp(w *bufio.Writer, resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	w.Write(data)
	w.WriteByte('\n')
}
