package report

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var assertPattern = regexp.MustCompile(`^\s*([a-z_0-9]+)\s*(>=|<=|>|<|==)\s*([0-9.]+)\s*(%|ms|s)?\s*$`)

type AssertionResult struct {
	Expr   string
	Passed bool
	Actual float64
}

// EvaluateAssertions parses and checks each expression from the scenario's
// `assertions:` block (docs/dsl-reference.md) against an aggregated Result.
func EvaluateAssertions(exprs []string, r Result) ([]AssertionResult, error) {
	results := make([]AssertionResult, 0, len(exprs))
	for _, expr := range exprs {
		m := assertPattern.FindStringSubmatch(expr)
		if m == nil {
			return nil, fmt.Errorf("assertion: cannot parse %q", expr)
		}
		metric, op, valueStr, unit := m[1], m[2], m[3], m[4]
		threshold, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return nil, fmt.Errorf("assertion: invalid threshold in %q: %w", expr, err)
		}
		actual, err := metricValue(metric, r, unit)
		if err != nil {
			return nil, fmt.Errorf("assertion %q: %w", expr, err)
		}
		results = append(results, AssertionResult{
			Expr:   expr,
			Passed: compare(actual, op, threshold),
			Actual: actual,
		})
	}
	return results, nil
}

func metricValue(name string, r Result, unit string) (float64, error) {
	switch name {
	case "transaction_success_rate":
		return r.SuccessRate, nil
	case "rpc_p50":
		return durationInUnit(r.RPCLatency.P50, unit), nil
	case "rpc_p95":
		return durationInUnit(r.RPCLatency.P95, unit), nil
	case "rpc_p99":
		return durationInUnit(r.RPCLatency.P99, unit), nil
	case "confirmation_p50":
		return durationInUnit(r.TransactionLatency.P50, unit), nil
	case "confirmation_p95":
		return durationInUnit(r.TransactionLatency.P95, unit), nil
	case "confirmation_p99":
		return durationInUnit(r.TransactionLatency.P99, unit), nil
	case "gas_used_p95":
		return float64(r.Gas.P95), nil
	case "reverted_transactions":
		return float64(r.RevertedTransactions), nil
	case "nonce_errors":
		return float64(r.NonceErrors), nil
	case "rpc_errors":
		return float64(r.RPCErrors), nil
	default:
		return 0, fmt.Errorf("unknown metric %q", name)
	}
}

func durationInUnit(d time.Duration, unit string) float64 {
	if unit == "s" {
		return d.Seconds()
	}
	return float64(d.Milliseconds())
}

func compare(actual float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return actual > threshold
	case "<":
		return actual < threshold
	case ">=":
		return actual >= threshold
	case "<=":
		return actual <= threshold
	case "==":
		return actual == threshold
	default:
		return false
	}
}
