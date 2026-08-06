// Package distributed implements Web3Load's controller/worker mode: one
// controller process shards a scenario and its wallets across N worker
// processes, each of which runs the exact same internal/load.Engine a
// single-process `run` would, against the same chain, and streams its
// progress back for aggregation.
//
// The wire protocol is plain JSON over HTTP, not gRPC — a deliberate v0.3
// simplification (see docs/distributed.md) rather than the gRPC design
// sketched in the original architecture doc: it needed no code-generation
// toolchain to build and verify, at the cost of a slightly less compact
// wire format that doesn't matter at this message volume (one call per
// worker registration, one call per progress interval).
package distributed

import (
	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/wallet"
)

// RegisterRequest is sent once by a worker on startup.
type RegisterRequest struct {
	WorkerID string `json:"worker_id"`
}

// Assignment is the controller's response to a successful registration:
// this worker's slice of the scenario and wallets to run. The scenario
// travels as YAML text (not a path) so workers never need filesystem
// access to anything the controller can see — see
// scenario.Scenario's MarshalYAML/UnmarshalYAML round-trip.
type Assignment struct {
	ShardIndex   int             `json:"shard_index"`
	ShardCount   int             `json:"shard_count"`
	ScenarioYAML string          `json:"scenario_yaml"`
	Wallets      []wallet.Wallet `json:"wallets"`
}

// MetricsReport is pushed by a worker periodically (Done=false) and once
// more at the end of its run (Done=true).
type MetricsReport struct {
	WorkerID string        `json:"worker_id"`
	Result   report.Result `json:"result"`
	Done     bool          `json:"done"`
}

// MetricsAck is an intentionally minimal response — the protocol doesn't
// need the controller to say anything back beyond "received."
type MetricsAck struct {
	OK bool `json:"ok"`
}
