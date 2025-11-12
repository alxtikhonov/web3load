package report

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteConsole_MatchesTargetLayout(t *testing.T) {
	r := Result{
		Duration:              60 * time.Second,
		TotalTransactions:     100000,
		Throughput:            1666,
		SuccessRate:           99.82,
		RPCLatency:            Percentiles{P50: 42 * time.Millisecond, P95: 180 * time.Millisecond, P99: 420 * time.Millisecond},
		TransactionLatency:    Percentiles{P50: 1800 * time.Millisecond, P95: 4200 * time.Millisecond, P99: 8700 * time.Millisecond},
		Gas:                   GasStats{Average: 142312, P95: 189420},
		RevertedTransactions:  180,
		NonceErrors:           12,
		RPCErrors:             8,
	}

	var buf bytes.Buffer
	WriteConsole(&buf, r)
	out := buf.String()

	for _, want := range []string{
		"100,000", "1,666 TPS", "99.82%",
		"42 ms", "180 ms", "420 ms",
		"1.8 s", "4.2 s", "8.7 s",
		"142,312", "189,420",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("console report missing %q\n---\n%s", want, out)
		}
	}
}
