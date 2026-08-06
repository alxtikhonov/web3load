package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/wallet"
)

// Controller shards a scenario and wallet set across NumWorkers and
// aggregates their results. It never runs any load itself — pure
// orchestration, same responsibility split as the architecture doc.
type Controller struct {
	Scenario   *scenario.Scenario
	Wallets    []wallet.Wallet
	NumWorkers int

	mu         sync.Mutex
	registered []string
	ready      chan struct{}
	readyOnce  sync.Once

	resultsMu   sync.Mutex
	results     map[string]report.Result
	doneWorkers map[string]bool
	allDone     chan struct{}
	doneOnce    sync.Once
}

func NewController(s *scenario.Scenario, wallets []wallet.Wallet, numWorkers int) *Controller {
	return &Controller{
		Scenario:    s,
		Wallets:     wallets,
		NumWorkers:  numWorkers,
		ready:       make(chan struct{}),
		results:     make(map[string]report.Result),
		doneWorkers: make(map[string]bool),
		allDone:     make(chan struct{}),
	}
}

func (c *Controller) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", c.handleRegister)
	mux.HandleFunc("/metrics", c.handleMetrics)
	return mux
}

// handleRegister blocks each caller until exactly NumWorkers have
// registered, then releases them all together with their shard — a simple
// barrier that keeps every worker's ramp/stage timing starting from
// roughly the same moment instead of drifting apart by however long
// workers take to start up and connect.
func (c *Controller) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	idx := len(c.registered)
	c.registered = append(c.registered, req.WorkerID)
	slog.Info("controller: worker registered", "worker_id", req.WorkerID, "shard_index", idx, "have", len(c.registered), "want", c.NumWorkers)
	if len(c.registered) >= c.NumWorkers {
		c.readyOnce.Do(func() { close(c.ready) })
	}
	c.mu.Unlock()

	select {
	case <-c.ready:
	case <-r.Context().Done():
		return
	}

	sharded, err := c.Scenario.Load.Shard(idx, c.NumWorkers)
	if err != nil {
		http.Error(w, fmt.Sprintf("sharding load: %v", err), http.StatusInternalServerError)
		return
	}
	shardScenario := *c.Scenario
	shardScenario.Load = sharded

	data, err := yaml.Marshal(&shardScenario)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshaling shard scenario: %v", err), http.StatusInternalServerError)
		return
	}

	resp := Assignment{
		ShardIndex:   idx,
		ShardCount:   c.NumWorkers,
		ScenarioYAML: string(data),
		Wallets:      wallet.ShardWallets(c.Wallets, idx, c.NumWorkers),
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("controller: failed to write assignment response", "worker_id", req.WorkerID, "error", err)
	}
}

func (c *Controller) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var req MetricsReport
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.resultsMu.Lock()
	c.results[req.WorkerID] = req.Result
	if req.Done {
		c.doneWorkers[req.WorkerID] = true
		if len(c.doneWorkers) >= c.NumWorkers {
			c.doneOnce.Do(func() { close(c.allDone) })
		}
	}
	c.resultsMu.Unlock()

	slog.Info("controller: metrics received", "worker_id", req.WorkerID, "done", req.Done, "transactions", req.Result.TotalTransactions)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(MetricsAck{OK: true})
}

// WaitForCompletion blocks until every registered worker has reported
// Done=true, or ctx is cancelled first.
func (c *Controller) WaitForCompletion(ctx context.Context) error {
	select {
	case <-c.allDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Aggregate combines each worker's latest snapshot into one report.Result.
// Counts sum exactly; success rate is recomputed from the summed counts
// (weighted by each worker's volume, not a plain average across workers,
// so an idle worker can't skew it). Latency percentiles cannot be merged
// exactly from other percentiles without the underlying samples — this
// takes the max p50/p95/p99 across workers as a conservative (pessimistic)
// approximation, not a mathematically exact merged percentile. That's a
// documented limitation, not a silent inaccuracy: see docs/distributed.md.
func (c *Controller) Aggregate() report.Result {
	c.resultsMu.Lock()
	defer c.resultsMu.Unlock()

	var agg report.Result
	if c.Scenario != nil {
		agg.ScenarioName = c.Scenario.Info.Name
	}

	var weightedSuccess float64
	for _, r := range c.results {
		agg.TotalTransactions += r.TotalTransactions
		agg.Throughput += r.Throughput
		agg.RevertedTransactions += r.RevertedTransactions
		agg.NonceErrors += r.NonceErrors
		agg.RPCErrors += r.RPCErrors
		if r.Gas.P95 > agg.Gas.P95 {
			agg.Gas.P95 = r.Gas.P95
		}
		if r.Gas.Average > agg.Gas.Average {
			agg.Gas.Average = r.Gas.Average
		}
		agg.RPCLatency = maxPercentiles(agg.RPCLatency, r.RPCLatency)
		agg.TransactionLatency = maxPercentiles(agg.TransactionLatency, r.TransactionLatency)
		if r.Duration > agg.Duration {
			agg.Duration = r.Duration
		}
		weightedSuccess += r.SuccessRate * float64(r.TotalTransactions)
	}
	if agg.TotalTransactions > 0 {
		agg.SuccessRate = weightedSuccess / float64(agg.TotalTransactions)
	}
	return agg
}

func maxPercentiles(a, b report.Percentiles) report.Percentiles {
	return report.Percentiles{
		P50: maxDuration(a.P50, b.P50),
		P95: maxDuration(a.P95, b.P95),
		P99: maxDuration(a.P99, b.P99),
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
