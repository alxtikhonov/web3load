// Package metrics collects run-time counters and latency samples from every
// virtual user and turns them into the aggregated report.Result the console
// reporter, JSON export, and assertions all consume. In-memory sample
// storage (rather than a streaming quantile estimator) is a deliberate v0.1
// simplification, fine at MVP scale and flagged in the roadmap for v0.2.
package metrics

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/report"
)

type Collector struct {
	mu        sync.Mutex
	startedAt time.Time

	successSteps    int64
	revertedSteps   int64
	errorsByKind    map[string]int64
	submitLatencies []time.Duration
	confirmLatencies []time.Duration
	gasUsed          []uint64
}

func New() *Collector {
	return &Collector{
		startedAt:    time.Now(),
		errorsByKind: make(map[string]int64),
	}
}

// RecordStep is called once per executed action step, from any virtual
// user's goroutine — it must be safe under concurrent use. wallClock (total
// step time including think/wait) is accepted but not yet broken out as its
// own metric; reserved for a v0.2 per-step timing view.
func (c *Collector) RecordStep(actionName string, res action.Result, err error, wallClock time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if res.SubmitLatency > 0 {
		c.submitLatencies = append(c.submitLatencies, res.SubmitLatency)
	}
	if res.ConfirmLatency > 0 {
		c.confirmLatencies = append(c.confirmLatencies, res.ConfirmLatency)
	}
	if res.GasUsed > 0 {
		c.gasUsed = append(c.gasUsed, res.GasUsed)
	}

	switch {
	case err != nil:
		c.errorsByKind[classifyError(err)]++
	case res.RevertReason != "":
		c.revertedSteps++
	case res.Success:
		c.successSteps++
	}
}

func (c *Collector) RecordError(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errorsByKind[kind]++
}

func classifyError(err error) string {
	msg := err.Error()
	switch {
	case containsAny(msg, "nonce too low", "nonce too high"):
		return "nonce_error"
	case containsAny(msg, "not confirmed within"):
		return "timeout"
	default:
		return "rpc_error"
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Snapshot aggregates everything recorded so far into a report.Result. It
// can be called mid-run (for the Prometheus exporter) or once at the end.
func (c *Collector) Snapshot(scenarioName string) report.Result {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.startedAt)
	total := c.successSteps + c.revertedSteps + sumErrors(c.errorsByKind)

	var successRate float64
	if total > 0 {
		successRate = float64(c.successSteps) / float64(total) * 100
	}
	var throughput float64
	if elapsed.Seconds() > 0 {
		throughput = float64(total) / elapsed.Seconds()
	}

	return report.Result{
		ScenarioName:         scenarioName,
		Duration:             elapsed,
		TotalTransactions:    total,
		Throughput:           throughput,
		SuccessRate:          successRate,
		RPCLatency:           percentilesOf(c.submitLatencies),
		TransactionLatency:   percentilesOf(c.confirmLatencies),
		Gas:                  gasStatsOf(c.gasUsed),
		RevertedTransactions: c.revertedSteps,
		NonceErrors:          c.errorsByKind["nonce_error"],
		RPCErrors:            c.errorsByKind["rpc_error"],
	}
}

func sumErrors(m map[string]int64) int64 {
	var total int64
	for _, v := range m {
		total += v
	}
	return total
}

func percentilesOf(samples []time.Duration) report.Percentiles {
	if len(samples) == 0 {
		return report.Percentiles{}
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return report.Percentiles{
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) * p) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func gasStatsOf(samples []uint64) report.GasStats {
	if len(samples) == 0 {
		return report.GasStats{}
	}
	sorted := append([]uint64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var sum uint64
	for _, v := range sorted {
		sum += v
	}
	idx := (len(sorted) * 95) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return report.GasStats{
		Average: float64(sum) / float64(len(sorted)),
		P95:     sorted[idx],
	}
}
