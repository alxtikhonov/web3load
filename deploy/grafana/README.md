# Grafana

Point Grafana (started via `../docker/demo-compose.yaml`, http://localhost:3000)
at `http://prometheus:9090` as a Prometheus data source, then graph the
gauges exposed by `web3load run --metrics-addr :9090`:

- `web3load_success_rate_percent`
- `web3load_throughput_tps`
- `web3load_rpc_latency_p95_ms`
- `web3load_confirmation_latency_p95_ms`
- `web3load_reverted_transactions_total`
- `web3load_nonce_errors_total`
- `web3load_rpc_errors_total`

A pre-built importable dashboard JSON is a v0.2 roadmap item — shipping one
now would mean maintaining panel definitions no one has validated against a
real run yet. Building it from the gauges above once the metric set has
proven itself in practice is the more honest sequencing.
