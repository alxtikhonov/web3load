# Grafana

```bash
web3load run scenario.yaml --metrics-addr :9090 &
docker compose -f ../docker/demo-compose.yaml up
```

Open http://localhost:3000 — the **Web3Load Overview** dashboard is
auto-provisioned (no manual import, no datasource setup): `provisioning/`
registers Prometheus (pointed at the `prometheus` compose service) and the
dashboard folder, and `dashboards/web3load-overview.json` is loaded from
there. Anonymous access is enabled for the demo (`GF_AUTH_ANONYMOUS_*` in
`../docker/demo-compose.yaml`) — don't reuse that compose file as-is for
anything but a local demo.

## What's on it

Five panels, all reading the gauges `web3load run --metrics-addr` exports:

| Panel | Metric |
|---|---|
| Throughput | `web3load_throughput_tps` |
| Success Rate | `web3load_success_rate_percent` (thresholds at 95%/99%) |
| RPC Latency (p95) | `web3load_rpc_latency_p95_ms` |
| Confirmation Latency (p95) | `web3load_confirmation_latency_p95_ms` |
| Errors | `web3load_reverted_transactions_total`, `web3load_nonce_errors_total`, `web3load_rpc_errors_total` |

## Using it outside the demo compose file

Point your own Prometheus at `web3load run`'s `--metrics-addr`, add it as a
Grafana data source named `Prometheus` (or edit the `datasource` field in
`dashboards/web3load-overview.json` to match your data source's name), and
import `dashboards/web3load-overview.json` directly via Grafana's
Dashboards → Import screen.
