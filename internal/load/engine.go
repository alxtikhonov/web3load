// Package load drives the configured load model (constant or ramping in
// v0.1), spawning and retiring virtual-user goroutines to match the target
// concurrency over time, and runs each VU's step loop against its assigned
// wallet.
package load

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/metrics"
	"github.com/web3load/web3load/internal/scenario"
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
	switch e.Scenario.Load.Type {
	case "constant":
		return e.runConstant(ctx)
	case "ramping":
		return e.runRamping(ctx)
	default:
		return fmt.Errorf("load: unsupported load type %q", e.Scenario.Load.Type)
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
// assigned wallets.
func (e *Engine) runRamping(ctx context.Context) error {
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

	for _, stage := range e.Scenario.Load.Stages {
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
			return
		}

		start := time.Now()
		res, err := act.Execute(ctx, e.Deps, step, state)
		e.Metrics.RecordStep(step.Action, res, err, time.Since(start))

		if step.SaveAs != "" && res.Value != nil {
			state.Saved[step.SaveAs] = res.Value
		}
		if err != nil {
			return
		}
	}
}
