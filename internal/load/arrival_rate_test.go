package load

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/metrics"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/wallet"
)

type noopAction struct{}

func (noopAction) Execute(ctx context.Context, deps action.Deps, step scenario.Step, state *action.VUState) (action.Result, error) {
	return action.Result{Success: true}, nil
}

type slowAction struct{ delay time.Duration }

func (a slowAction) Execute(ctx context.Context, deps action.Deps, step scenario.Step, state *action.VUState) (action.Result, error) {
	time.Sleep(a.delay)
	return action.Result{Success: true}, nil
}

func init() {
	action.Register("test_noop", func() action.Action { return noopAction{} })
	action.Register("test_slow_50ms", func() action.Action { return slowAction{delay: 50 * time.Millisecond} })
}

func TestRunArrivalRate_RunsApproximatelyRateTimesDuration(t *testing.T) {
	s := &scenario.Scenario{
		Info: scenario.Info{Name: "arrival_test"},
		Load: scenario.Load{
			Type: "arrival-rate", Rate: 50, TimeUnit: scenario.Duration(time.Second),
			MaxVUs: 100, Duration: scenario.Duration(200 * time.Millisecond),
		},
		Steps: []scenario.Step{{Action: "test_noop"}},
	}
	wallets := []wallet.Wallet{{Address: common.HexToAddress("0x1")}}
	m := metrics.New()
	e := New(s, wallets, action.Deps{}, m)

	start := time.Now()
	if err := e.runArrivalRate(context.Background()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond || elapsed > 600*time.Millisecond {
		t.Fatalf("expected runArrivalRate to run for ~200ms, took %s", elapsed)
	}

	// Rate=50/s for 200ms => ~10 iterations expected; generous tolerance for
	// scheduler/ticker jitter, especially in CI.
	snap := m.Snapshot("arrival_test")
	if snap.TotalTransactions < 4 || snap.TotalTransactions > 20 {
		t.Fatalf("expected roughly 10 iterations, got %d", snap.TotalTransactions)
	}
}

func TestRunArrivalRate_DropsIterationsWhenMaxVUsExhausted(t *testing.T) {
	s := &scenario.Scenario{
		Info: scenario.Info{Name: "arrival_drop_test"},
		Load: scenario.Load{
			// 100/s = one tick every 10ms, but MaxVUs=1 forces iterations to
			// run one at a time and each takes 50ms, so most ticks over the
			// 200ms window must be dropped rather than queued.
			Type: "arrival-rate", Rate: 100, TimeUnit: scenario.Duration(time.Second),
			MaxVUs: 1, Duration: scenario.Duration(200 * time.Millisecond),
		},
		Steps: []scenario.Step{{Action: "test_slow_50ms"}},
	}
	wallets := []wallet.Wallet{{Address: common.HexToAddress("0x2")}}
	m := metrics.New()
	e := New(s, wallets, action.Deps{}, m)

	if err := e.runArrivalRate(context.Background()); err != nil {
		t.Fatal(err)
	}

	// At most ~4-5 serialized 50ms iterations fit in 200ms; if drops weren't
	// happening, up to ~20 ticks would each have queued/run their own
	// goroutine and this would be far higher.
	snap := m.Snapshot("arrival_drop_test")
	if snap.TotalTransactions == 0 || snap.TotalTransactions > 8 {
		t.Fatalf("expected roughly 4 serialized iterations (proving excess ticks were dropped, not queued), got %d", snap.TotalTransactions)
	}
}
