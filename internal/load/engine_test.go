package load

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/wallet"
)

// countingAction returns a scripted sequence of (result, error) pairs, one
// per call, repeating the last pair once the script is exhausted — enough
// to drive executeWithRetry's decision logic without a real chain.
type countingAction struct {
	calls   int
	results []action.Result
	errs    []error
}

func (a *countingAction) Execute(ctx context.Context, deps action.Deps, step scenario.Step, state *action.VUState) (action.Result, error) {
	i := a.calls
	if i >= len(a.results) {
		i = len(a.results) - 1
	}
	a.calls++
	return a.results[i], a.errs[i]
}

func TestExecuteWithRetry_RetriesPreBroadcastFailure(t *testing.T) {
	act := &countingAction{
		results: []action.Result{{}, {Success: true}},
		errs:    []error{errors.New("transient rpc error"), nil},
	}
	e := &Engine{}
	step := scenario.Step{Action: "get_balance", Retry: &scenario.RetryPolicy{MaxAttempts: 3}}
	w := wallet.Wallet{Address: common.HexToAddress("0x1")}
	state := &action.VUState{Wallet: w}

	res, err := e.executeWithRetry(context.Background(), act, step, state, w)
	if err != nil {
		t.Fatalf("expected eventual success, got error: %v", err)
	}
	if !res.Success {
		t.Fatal("expected a successful result after retry")
	}
	if act.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", act.calls)
	}
}

func TestExecuteWithRetry_DoesNotRetryAfterBroadcast(t *testing.T) {
	// The step already produced a tx_hash, so the "failure" here is a
	// confirmation timeout after successful submission — retrying would
	// risk sending a second, conflicting transaction.
	act := &countingAction{
		results: []action.Result{{TxHash: "0xabc"}},
		errs:    []error{errors.New("confirmation timeout")},
	}
	e := &Engine{}
	step := scenario.Step{Action: "transfer", Retry: &scenario.RetryPolicy{MaxAttempts: 5}}
	w := wallet.Wallet{Address: common.HexToAddress("0x2")}
	state := &action.VUState{Wallet: w}

	_, err := e.executeWithRetry(context.Background(), act, step, state, w)
	if err == nil {
		t.Fatal("expected the error to propagate")
	}
	if act.calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry once broadcast), got %d", act.calls)
	}
}

func TestExecuteWithRetry_StopsAtMaxAttempts(t *testing.T) {
	act := &countingAction{
		results: []action.Result{{}},
		errs:    []error{errors.New("still failing")},
	}
	e := &Engine{}
	step := scenario.Step{Action: "get_balance", Retry: &scenario.RetryPolicy{MaxAttempts: 3, BaseDelay: scenario.Duration(time.Millisecond)}}
	w := wallet.Wallet{Address: common.HexToAddress("0x3")}
	state := &action.VUState{Wallet: w}

	_, err := e.executeWithRetry(context.Background(), act, step, state, w)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if act.calls != 3 {
		t.Fatalf("expected 3 calls (max_attempts), got %d", act.calls)
	}
}

func TestExecuteWithRetry_NilPolicyMeansSingleAttempt(t *testing.T) {
	act := &countingAction{
		results: []action.Result{{}},
		errs:    []error{errors.New("fails")},
	}
	e := &Engine{}
	step := scenario.Step{Action: "get_balance"} // Retry is nil
	w := wallet.Wallet{Address: common.HexToAddress("0x4")}
	state := &action.VUState{Wallet: w}

	_, err := e.executeWithRetry(context.Background(), act, step, state, w)
	if err == nil {
		t.Fatal("expected an error")
	}
	if act.calls != 1 {
		t.Fatalf("expected exactly 1 call with no retry policy, got %d", act.calls)
	}
}
