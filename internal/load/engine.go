// Package load drives the configured load model, spawning and retiring
// virtual-user goroutines to match the target concurrency over time, and
// runs each VU's step loop against its assigned wallet. constant and soak
// share one execution path (a fixed VU count held for a duration); ramping,
// spike, and stress share another (a sequence of stages) — spike/stress are
// expanded into stages by scenario.Load.ResolvedStages before they ever
// reach this package. arrival-rate is the one genuinely different
// scheduler: it controls the rate new iterations start, not how many run
// concurrently, so it can't be expressed as either of the other two.
package load

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/metrics"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/telemetry"
	"github.com/web3load/web3load/internal/wallet"
)

type Engine struct {
	Scenario *scenario.Scenario
	Wallets  []wallet.Wallet
	Deps     action.Deps
	Metrics  *metrics.Collector

	walletCursor int64
}

func New(s *scenario.Scenario, wallets []wallet.Wallet, deps action.Deps, m *metrics.Collector) *Engine {
	return &Engine{Scenario: s, Wallets: wallets, Deps: deps, Metrics: m}
}

func (e *Engine) Run(ctx context.Context) error {
	if len(e.Wallets) == 0 {
		return fmt.Errorf("load: no wallets available")
	}

	loadType := e.Scenario.Load.Type
	slog.Info("load: run starting", "scenario", e.Scenario.Info.Name, "load_type", loadType, "wallets", len(e.Wallets))
	defer slog.Info("load: run finished", "scenario", e.Scenario.Info.Name)

	switch loadType {
	case "constant", "soak":
		return e.runConstant(ctx)
	case "ramping", "spike", "stress":
		stages, err := e.Scenario.Load.ResolvedStages()
		if err != nil {
			return err
		}
		return e.runRamping(ctx, stages)
	case "arrival-rate":
		return e.runArrivalRate(ctx)
	default:
		return fmt.Errorf("load: unsupported load type %q", loadType)
	}
}

func (e *Engine) runConstant(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, e.Scenario.Load.Duration.AsTime())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < e.Scenario.Load.VUs; i++ {
		wg.Add(1)
		go e.spawnVU(ctx, &wg, e.nextWallet())
	}
	wg.Wait()

	if err := ctx.Err(); err != nil && err != context.DeadlineExceeded {
		return err
	}
	return nil
}

// runRamping moves the active VU count to each stage's target at the start
// of the stage, holds it for the stage's duration, then moves to the next
// stage. Scaling down retires the most recently spawned VUs first (a
// simple LIFO policy), and scaling up spawns fresh ones against newly
// assigned wallets. It's shared by ramping, spike, and stress — they only
// differ in how their stages were produced.
func (e *Engine) runRamping(ctx context.Context, stages []scenario.Stage) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	active := make([]context.CancelFunc, 0)

	scaleTo := func(target int) {
		mu.Lock()
		defer mu.Unlock()
		delta := target - len(active)
		if delta > 0 {
			for i := 0; i < delta; i++ {
				vuCtx, vuCancel := context.WithCancel(ctx)
				active = append(active, vuCancel)
				wg.Add(1)
				go e.spawnVU(vuCtx, &wg, e.nextWallet())
			}
		} else if delta < 0 {
			for i := 0; i < -delta && len(active) > 0; i++ {
				last := len(active) - 1
				active[last]()
				active = active[:last]
			}
		}
	}

	stopAll := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, stop := range active {
			stop()
		}
		active = nil
	}

	for i, stage := range stages {
		slog.Info("load: entering stage", "stage", i, "target_vus", stage.Target, "duration", stage.Duration.AsTime())
		scaleTo(stage.Target)
		select {
		case <-time.After(stage.Duration.AsTime()):
		case <-ctx.Done():
			stopAll()
			wg.Wait()
			return ctx.Err()
		}
	}

	stopAll()
	wg.Wait()
	return nil
}

// runArrivalRate starts Rate new iterations per TimeUnit, each running on
// its own goroutine, capped at MaxVUs concurrently in flight via a
// semaphore. Unlike the VU-count models above, an iteration here runs
// exactly once and exits — the "rate" is a property of how often new
// iterations start, decoupled from how long each one takes. If iterations
// are taking longer than the arrival interval and MaxVUs is already
// saturated, the tick is dropped rather than queued or blocked: a load
// generator that fell behind schedule and silently caught up later would
// misrepresent the rate it actually achieved.
func (e *Engine) runArrivalRate(ctx context.Context) error {
	l := e.Scenario.Load
	unit := l.TimeUnit.AsTime()
	if unit <= 0 {
		unit = time.Second
	}
	interval := unit / time.Duration(l.Rate)
	if interval <= 0 {
		interval = time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, l.Duration.AsTime())
	defer cancel()

	sem := make(chan struct{}, l.MaxVUs)
	var wg sync.WaitGroup
	var dropped int64

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			if dropped > 0 {
				slog.Warn("load: arrival-rate finished with dropped iterations", "dropped", dropped, "max_vus", l.MaxVUs)
			}
			if err := ctx.Err(); err != nil && err != context.DeadlineExceeded {
				return err
			}
			return nil
		case <-ticker.C:
			select {
			case sem <- struct{}{}:
				wg.Add(1)
				w := e.nextWallet()
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					e.runIteration(ctx, w)
				}()
			default:
				dropped++
				slog.Warn("load: arrival-rate dropped an iteration, max_vus exhausted", "max_vus", l.MaxVUs)
			}
		}
	}
}

func (e *Engine) nextWallet() wallet.Wallet {
	idx := atomic.AddInt64(&e.walletCursor, 1) - 1
	return e.Wallets[int(idx)%len(e.Wallets)]
}

func (e *Engine) spawnVU(ctx context.Context, wg *sync.WaitGroup, w wallet.Wallet) {
	defer wg.Done()
	for ctx.Err() == nil {
		e.runIteration(ctx, w)
	}
}

// runIteration executes the scenario's steps once for wallet w. A step
// error abandons the rest of the iteration (the VU starts a fresh
// iteration from the first step) rather than retrying mid-flow, so a
// failure never leaves a VU stuck resubmitting a stale nonce/state.
func (e *Engine) runIteration(ctx context.Context, w wallet.Wallet) {
	state := &action.VUState{
		Wallet:  w,
		Vars:    e.Scenario.Vars,
		Saved:   make(map[string]interface{}),
		ChainID: e.Scenario.Target.ChainID,
	}

	for _, step := range e.Scenario.Steps {
		if ctx.Err() != nil {
			return
		}
		if step.Action == "" {
			select {
			case <-time.After(step.Think.AsTime()):
			case <-ctx.Done():
			}
			continue
		}

		act, ok := action.Get(step.Action)
		if !ok {
			e.Metrics.RecordError("unknown_action")
			slog.Error("load: unknown action", "action", step.Action, "wallet", w.Address.Hex())
			return
		}

		start := time.Now()
		res, err := e.executeWithRetry(ctx, act, step, state, w)
		e.Metrics.RecordStep(step.Action, res, err, time.Since(start))

		if step.SaveAs != "" && res.Value != nil {
			state.Saved[step.SaveAs] = res.Value
		}
		if err != nil {
			slog.Warn("load: step failed", "action", step.Action, "wallet", w.Address.Hex(), "error", err)
			return
		}
		if res.RevertReason != "" {
			slog.Warn("load: transaction reverted", "action", step.Action, "wallet", w.Address.Hex(), "tx_hash", res.TxHash)
		}
		slog.Debug("load: step ok", "action", step.Action, "wallet", w.Address.Hex(), "tx_hash", res.TxHash)
	}
}

// executeWithRetry runs one step, retrying per step.Retry (default: no
// retry) with exponential backoff. It only retries a failure that left no
// tx_hash behind — meaning nothing was ever broadcast — because retrying
// after a transaction is already in the mempool could double-send. A
// confirmation timeout after successful submission is therefore never
// retried here regardless of the configured policy.
func (e *Engine) executeWithRetry(ctx context.Context, act action.Action, step scenario.Step, state *action.VUState, w wallet.Wallet) (action.Result, error) {
	maxAttempts := 1
	var baseDelay time.Duration
	if step.Retry != nil {
		maxAttempts = step.Retry.MaxAttempts
		baseDelay = step.Retry.BaseDelay.AsTime()
	}

	var res action.Result
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		spanCtx, span := telemetry.StartStep(ctx, step.Action, w.Address.Hex())
		res, err = act.Execute(spanCtx, e.Deps, step, state)
		telemetry.RecordOutcome(span, res.TxHash, res.GasUsed, err)

		if err == nil || res.TxHash != "" || attempt == maxAttempts-1 {
			return res, err
		}

		delay := baseDelay * time.Duration(int64(1)<<uint(attempt))
		slog.Warn("load: step failed, retrying", "action", step.Action, "wallet", w.Address.Hex(),
			"attempt", attempt+1, "max_attempts", maxAttempts, "delay", delay, "error", err)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return res, ctx.Err()
		}
	}
	return res, err
}
