package report

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func WriteConsole(w io.Writer, r Result) {
	fmt.Fprintln(w, "Web3 Load Test")
	fmt.Fprintln(w, "────────────────────────────")
	fmt.Fprintln(w)
	line(w, "Duration", formatDuration(r.Duration))
	line(w, "Transactions", formatInt(r.TotalTransactions))
	line(w, "Throughput", fmt.Sprintf("%s TPS", formatFloat(r.Throughput)))
	fmt.Fprintln(w)
	line(w, "Success rate", fmt.Sprintf("%.2f%%", r.SuccessRate))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "RPC latency")
	line(w, "  p50", formatDuration(r.RPCLatency.P50))
	line(w, "  p95", formatDuration(r.RPCLatency.P95))
	line(w, "  p99", formatDuration(r.RPCLatency.P99))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Transaction latency")
	line(w, "  p50", formatDuration(r.TransactionLatency.P50))
	line(w, "  p95", formatDuration(r.TransactionLatency.P95))
	line(w, "  p99", formatDuration(r.TransactionLatency.P99))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Gas")
	line(w, "  average", formatInt(int64(r.Gas.Average)))
	line(w, "  p95", formatInt(int64(r.Gas.P95)))
	fmt.Fprintln(w)
	line(w, "Reverted transactions", formatInt(r.RevertedTransactions))
	line(w, "Nonce errors", formatInt(r.NonceErrors))
	line(w, "RPC errors", formatInt(r.RPCErrors))
}

func line(w io.Writer, label, value string) {
	fmt.Fprintf(w, "%-24s%12s\n", label, value)
}

func formatDuration(d time.Duration) string {
	switch {
	case d == 0:
		return "-"
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
}

func formatInt(n int64) string {
	return addThousands(fmt.Sprintf("%d", n))
}

func formatFloat(f float64) string {
	return addThousands(fmt.Sprintf("%.0f", f))
}

func addThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	n := len(s)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var out []byte
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
