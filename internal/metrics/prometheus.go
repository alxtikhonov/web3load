package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusExporter exposes a Collector's periodically-refreshed snapshot
// as a /metrics endpoint, for scraping alongside Grafana dashboards while a
// scenario is still running.
type PrometheusExporter struct {
	collector *Collector
	scenario  string
	mux       *http.ServeMux

	successRate       prometheus.Gauge
	throughput        prometheus.Gauge
	rpcLatencyP95     prometheus.Gauge
	confirmLatencyP95 prometheus.Gauge
	reverted          prometheus.Gauge
	nonceErrors       prometheus.Gauge
	rpcErrors         prometheus.Gauge
}

func NewPrometheusExporter(c *Collector, scenarioName string) *PrometheusExporter {
	e := &PrometheusExporter{
		collector:         c,
		scenario:          scenarioName,
		successRate:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_success_rate_percent", Help: "Rolling transaction success rate."}),
		throughput:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_throughput_tps", Help: "Transactions per second."}),
		rpcLatencyP95:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_rpc_latency_p95_ms", Help: "p95 submission latency in ms."}),
		confirmLatencyP95: prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_confirmation_latency_p95_ms", Help: "p95 confirmation latency in ms."}),
		reverted:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_reverted_transactions_total", Help: "Reverted transaction count."}),
		nonceErrors:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_nonce_errors_total", Help: "Nonce-related error count."}),
		rpcErrors:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "web3load_rpc_errors_total", Help: "RPC error count."}),
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(e.successRate, e.throughput, e.rpcLatencyP95, e.confirmLatencyP95, e.reverted, e.nonceErrors, e.rpcErrors)

	e.mux = http.NewServeMux()
	e.mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return e
}

// Serve refreshes the exported gauges from the collector once per interval
// and serves /metrics on addr until ctx is cancelled.
func (e *PrometheusExporter) Serve(ctx context.Context, addr string, interval time.Duration) error {
	srv := &http.Server{Addr: addr, Handler: e.mux}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return srv.Shutdown(context.Background())
		case err := <-errCh:
			if err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("metrics: prometheus server: %w", err)
			}
			return nil
		case <-ticker.C:
			e.refresh()
		}
	}
}

func (e *PrometheusExporter) refresh() {
	snap := e.collector.Snapshot(e.scenario)
	e.successRate.Set(snap.SuccessRate)
	e.throughput.Set(snap.Throughput)
	e.rpcLatencyP95.Set(float64(snap.RPCLatency.P95.Milliseconds()))
	e.confirmLatencyP95.Set(float64(snap.TransactionLatency.P95.Milliseconds()))
	e.reverted.Set(float64(snap.RevertedTransactions))
	e.nonceErrors.Set(float64(snap.NonceErrors))
	e.rpcErrors.Set(float64(snap.RPCErrors))
}
