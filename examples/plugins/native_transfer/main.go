// Command native_transfer is an example Web3Load plugin demonstrating the
// "transaction" half of the plugin protocol: given args: [to, amount_wei],
// it returns a transaction spec for the host to build, sign, and submit.
// A trivial reimplementation of the built-in `transfer` action — useful
// only to prove the mechanism, not something you'd actually deploy a
// plugin for — see docs/plugins.md.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	Method string        `json:"method"`
	Args   []interface{} `json:"args"`
}

type response struct {
	Kind  string `json:"kind"`
	To    string `json:"to,omitempty"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
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
	if len(req.Args) < 2 {
		writeResp(w, response{Kind: "error", Error: "native_transfer: expected args: [to, amount_wei]"})
		return
	}
	to, ok := req.Args[0].(string)
	if !ok {
		writeResp(w, response{Kind: "error", Error: "native_transfer: args[0] (to) must be a string"})
		return
	}
	writeResp(w, response{Kind: "transaction", To: to, Value: fmt.Sprint(req.Args[1])})
}

func writeResp(w *bufio.Writer, resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	w.Write(data)
	w.WriteByte('\n')
}
