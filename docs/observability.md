# Observability

Three separate channels, deliberately kept apart:

- **stdout** — the report: `web3load run`'s console summary, `--out
  results.json`, and `PASS`/`FAIL` assertion lines. Meant to be piped or
  saved; nothing else writes here.
- **stderr** — structured logs: diagnostics from the run itself (nonce
  resyncs, per-step failures, stage transitions, progress snapshots).
- **Prometheus `/metrics` and OTLP spans** — for external tooling, opt-in
  via flags.

## Structured logs

Global flags (work on every subcommand):

```bash
web3load run scenario.yaml --log-level debug --log-format json
```

- `--log-level`: `debug` | `info` (default) | `warn` | `error`. `debug` logs
  every successful step (`action`, `wallet`, `tx_hash`) — very verbose at
  more than a handful of VUs, useful for chasing a specific wallet's
  behavior. `info` and above is what you want for a normal run.
- `--log-format`: `text` (default, human-readable) or `json` (for shipping
  to a log aggregator).

What gets logged, and at what level:

| Level | Examples |
|---|---|
| `error` | Unknown action referenced by a step (shouldn't happen past `validate`, but the engine doesn't trust that) |
| `warn` | A step failed (RPC error, timeout, revert); a nonce resync failed |
| `info` | Run start/finish, RPC connection established, stage transitions (spike/stress/ramping), a nonce resync succeeded, progress snapshots |
| `debug` | Every successful step |

## Progress snapshots

```bash
web3load run scenario.yaml --progress-interval 30s
```

Logs a compact `run: progress` line (elapsed time, TPS, success rate,
reverted/nonce/RPC error counts) at `info` level on a fixed interval,
independent of load type — most valuable for `soak` runs where the final
report might be hours away. `--progress-interval 0` disables it.

## Prometheus

```bash
web3load run scenario.yaml --metrics-addr :9090
```

See [README.md](../README.md#declarative-scenarios) and
[deploy/grafana/README.md](../deploy/grafana/README.md) for the exported
gauges and a Grafana setup.

## OpenTelemetry tracing

```bash
web3load run scenario.yaml --otel-endpoint localhost:4318
```

Exports one span per executed action step (`web3load.action.<name>`) over
OTLP/HTTP to the given collector (`host:port`, no scheme — TLS isn't used,
matching a local collector or sidecar; point it at a Docker Compose
`otel-collector` service or similar). Each span carries the acting wallet
address, and — once the step completes — the transaction hash and gas used
if any, with the error recorded on the span when the step failed.

Leaving `--otel-endpoint` unset is the default, zero-cost path: internally,
`internal/telemetry.StartStep` always calls `otel.Tracer(...).Start`, but
without a registered `TracerProvider` that resolves to the OpenTelemetry
API's built-in no-op implementation, so instrumentation doesn't need its own
on/off branch.

At thousands of concurrent VUs, one span per step across the whole run can
be a lot of trace volume — consider your collector's sampling configuration
if you're tracing a large `stress`/`soak` run rather than a smaller
diagnostic one.
