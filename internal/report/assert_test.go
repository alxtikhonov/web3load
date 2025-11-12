package report

import (
	"testing"
	"time"
)

func TestEvaluateAssertions(t *testing.T) {
	r := Result{
		SuccessRate:        99.82,
		RPCLatency:         Percentiles{P95: 180 * time.Millisecond},
		TransactionLatency: Percentiles{P95: 4200 * time.Millisecond},
	}

	results, err := EvaluateAssertions([]string{
		"transaction_success_rate > 99%",
		"rpc_p95 < 500ms",
		"confirmation_p95 < 30s",
		"rpc_p95 < 100ms",
	}, r)
	if err != nil {
		t.Fatal(err)
	}

	want := []bool{true, true, true, false}
	for i, w := range want {
		if results[i].Passed != w {
			t.Errorf("assertion %q: got passed=%v, want %v", results[i].Expr, results[i].Passed, w)
		}
	}
}

func TestEvaluateAssertions_UnknownMetric(t *testing.T) {
	_, err := EvaluateAssertions([]string{"mempool_size > 10"}, Result{})
	if err == nil {
		t.Fatal("expected an error for an unknown metric")
	}
}
