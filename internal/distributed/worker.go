package distributed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/chain/evm"
	"github.com/web3load/web3load/internal/load"
	"github.com/web3load/web3load/internal/metrics"
	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/txengine"
	"github.com/web3load/web3load/internal/wallet"
)

type WorkerConfig struct {
	ControllerURL    string
	WorkerID         string
	ProgressInterval time.Duration
	// HTTPClient is overridable for tests; production callers should leave
	// it nil (Run supplies a client with no timeout, since /register
	// intentionally blocks until the whole cohort has joined).
	HTTPClient *http.Client
}

// Run registers with the controller (blocking until the full cohort of
// workers has joined), runs this worker's shard of the scenario using the
// exact same internal/load.Engine a single-process `run` uses, and streams
// progress snapshots plus a final report back to the controller.
func Run(ctx context.Context, cfg WorkerConfig) error {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	assignment, err := register(ctx, client, cfg.ControllerURL, cfg.WorkerID)
	if err != nil {
		return fmt.Errorf("worker: register: %w", err)
	}
	slog.Info("worker: assignment received", "worker_id", cfg.WorkerID,
		"shard_index", assignment.ShardIndex, "shard_count", assignment.ShardCount, "wallets", len(assignment.Wallets))

	s, err := scenario.ParseYAML([]byte(assignment.ScenarioYAML))
	if err != nil {
		return fmt.Errorf("worker: parse assigned scenario: %w", err)
	}

	adapter, err := evm.Dial(ctx, s.Target.RPCURL, s.Target.ChainID)
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}

	nonces := wallet.NewNonceManager(adapter)
	engine := txengine.New(adapter, nonces)
	engine.MaxGasPriceGwei = s.Safety.MaxGasPriceGwei
	deps := action.Deps{Adapter: adapter, Engine: engine, Safety: s.Safety}

	collector := metrics.New()
	loadEngine := load.New(s, assignment.Wallets, deps, collector)

	if cfg.ProgressInterval > 0 {
		progressCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go pushProgress(progressCtx, client, cfg.ControllerURL, cfg.WorkerID, collector, s.Info.Name, cfg.ProgressInterval)
	}

	runErr := loadEngine.Run(ctx)

	final := collector.Snapshot(s.Info.Name)
	// The final report is sent best-effort with a fresh, short-lived
	// context: if the run was cancelled, ctx may already be done, but the
	// controller (and whoever's waiting on it) still deserves this
	// worker's last numbers rather than silence.
	reportCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := reportMetrics(reportCtx, client, cfg.ControllerURL, cfg.WorkerID, final, true); err != nil {
		slog.Warn("worker: failed to report final metrics", "worker_id", cfg.WorkerID, "error", err)
	}
	return runErr
}

func pushProgress(ctx context.Context, client *http.Client, controllerURL, workerID string, collector *metrics.Collector, scenarioName string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := collector.Snapshot(scenarioName)
			if err := reportMetrics(ctx, client, controllerURL, workerID, snap, false); err != nil {
				slog.Warn("worker: failed to push progress", "worker_id", workerID, "error", err)
			}
		}
	}
}

func register(ctx context.Context, client *http.Client, controllerURL, workerID string) (*Assignment, error) {
	body, err := json.Marshal(RegisterRequest{WorkerID: workerID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controllerURL+"/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("controller returned %s", resp.Status)
	}

	var a Assignment
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

func reportMetrics(ctx context.Context, client *http.Client, controllerURL, workerID string, result report.Result, done bool) error {
	body, err := json.Marshal(MetricsReport{WorkerID: workerID, Result: result, Done: done})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controllerURL+"/metrics", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("controller returned %s", resp.Status)
	}
	return nil
}
