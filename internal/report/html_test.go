package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteHTML_ContainsKeyFigures(t *testing.T) {
	r := Result{
		ScenarioName:       "dex_swap",
		Duration:           60 * time.Second,
		TotalTransactions:  1234,
		Throughput:         20.5,
		SuccessRate:        98.76,
		RPCLatency:         Percentiles{P50: 10 * time.Millisecond, P95: 20 * time.Millisecond, P99: 30 * time.Millisecond},
		TransactionLatency: Percentiles{P50: time.Second, P95: 2 * time.Second, P99: 3 * time.Second},
		Gas:                GasStats{Average: 100000, P95: 150000},
		RevertedTransactions: 5,
		NonceErrors:           1,
		RPCErrors:              2,
	}
	assertions := []AssertionResult{
		{Expr: "transaction_success_rate > 99%", Passed: false},
		{Expr: "rpc_p95 < 500ms", Passed: true},
	}

	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteHTML(path, r, assertions); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	for _, want := range []string{
		"dex_swap", "1,234", "98.76", "150,000",
		"transaction_success_rate &gt; 99%", "rpc_p95 &lt; 500ms",
		"FAIL", "PASS",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html report missing %q", want)
		}
	}
}

func TestWriteHTML_NilAssertionsOmitsSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteHTML(path, Result{ScenarioName: "no_assertions"}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Assertions</h2>") {
		t.Error("expected the Assertions section to be omitted when there are none")
	}
}
