// Package report renders aggregated run results as a console summary or
// JSON, and evaluates scenario assertions against them.
package report

import "time"

type Percentiles struct {
	P50 time.Duration `json:"p50"`
	P95 time.Duration `json:"p95"`
	P99 time.Duration `json:"p99"`
}

type GasStats struct {
	Average float64 `json:"average"`
	P95     uint64  `json:"p95"`
}

type Result struct {
	ScenarioName          string        `json:"scenario_name"`
	Duration               time.Duration `json:"duration"`
	TotalTransactions      int64         `json:"total_transactions"`
	Throughput              float64      `json:"throughput_tps"`
	SuccessRate             float64      `json:"success_rate"`
	RPCLatency               Percentiles `json:"rpc_latency"`
	TransactionLatency       Percentiles `json:"transaction_latency"`
	Gas                      GasStats    `json:"gas"`
	RevertedTransactions     int64       `json:"reverted_transactions"`
	NonceErrors              int64       `json:"nonce_errors"`
	RPCErrors                int64       `json:"rpc_errors"`
}
