# Distributed mode

One controller shards a scenario and its wallets across N worker
processes. Each worker runs the exact same `internal/load.Engine` a
single-process `web3load run` would, against the same chain, and streams
its progress back for aggregation — the controller never runs any load
itself, purely orchestration.

## Quickstart

```bash
# once, centrally
web3load wallets generate --count 3000 --out wallets.json
web3load wallets fund --wallets wallets.json --from <funder-key>

# controller
web3load controller run scenario.yaml --wallets wallets.json --workers 3 --listen :7700

# on however many machines, pointed at the controller
web3load worker --controller http://<controller-host>:7700
web3load worker --controller http://<controller-host>:7700
web3load worker --controller http://<controller-host>:7700
```

The controller blocks each worker's registration until all 3 have joined,
then releases them together (so their ramp/stage timing starts from
roughly the same moment) and hands each its shard: a disjoint slice of the
wallets and a scaled-down `Load` (see below). Every worker dials
`target.rpc_url` from the scenario itself — the controller doesn't proxy
chain traffic, so all workers need network access to the same RPC endpoint.

## How sharding works

- **Wallets**: split into `--workers` contiguous, non-overlapping slices
  (`wallet.ShardWallets`) — this is the hard invariant: no two workers may
  ever hold the same wallet, or their independently-tracked nonces for it
  would collide.
- **Load**: `scenario.Load.Shard` divides VU/rate targets evenly (any
  remainder goes to the earliest-indexed shards, so per-worker targets
  always sum back to the scenario's original numbers). `ramping`/`spike`/
  `stress` are first resolved to concrete stages, then each stage's target
  is divided — the shard a worker receives is always type `ramping`
  regardless of what the original scenario specified, since a divided
  stage list no longer matches the original spike/stress parameters.
  `arrival-rate` divides both `rate` and `max_vus`.

## Protocol: HTTP/JSON, not gRPC

The architecture doc sketched gRPC for this. v0.3 ships plain JSON over
HTTP instead — a deliberate, documented simplification: gRPC needs a
`protoc`/`protoc-gen-go` code-generation toolchain, which isn't a
reasonable thing to require just to coordinate a handful of worker
processes exchanging a few messages a second. Two endpoints on the
controller:

- `POST /register` — a worker sends its id; the handler blocks until
  `--workers` have all registered, then responds with that worker's
  `Assignment` (shard index/count, the scenario re-serialized as YAML text,
  and its wallet slice).
- `POST /metrics` — a worker pushes a `report.Result` snapshot, with
  `done: true` on its last one. `controller run` waits until every worker
  has reported done (or `--timeout` elapses) before aggregating.

## Aggregation is an approximation for latency

Transaction counts, throughput, reverted/nonce/RPC error counts, and
success rate combine exactly (success rate is recomputed from the summed
counts, weighted by each worker's volume — not a plain average across
workers, so an idle worker can't skew it). **Latency percentiles cannot be
merged exactly from other percentiles** without the underlying samples —
`Controller.Aggregate` takes the max p50/p95/p99 across workers as a
conservative (pessimistic) approximation. This is a documented limitation,
not a silent inaccuracy: if you need exact merged percentiles, aggregate
the raw JSON per-worker output yourself, or ship a streaming/approximate
histogram (t-digest or similar) — tracked as a v1.0-era improvement, not
done here.

## Security: no transport encryption or auth

The controller has no authentication and the channel has no TLS in v0.3 —
`Assignment` responses include wallet private keys, since a worker has to
sign transactions locally. **Run distributed mode only on a trusted
network** (a VPN, a private subnet, a Kubernetes namespace's internal
ClusterIP) — the same posture already required for a plaintext keystore
file (docs/security.md), extended to the network this data now also
crosses. Putting the controller's `--listen` address on a public interface
without a reverse proxy doing TLS+auth in front of it is not supported.

## What isn't here yet

- No worker auto-discovery — you point each worker at the controller
  explicitly.
- No mid-run worker failure recovery: if a worker dies, the controller's
  `--timeout` (if set) is what eventually gives up; there's no
  reassignment of its shard to another worker.
- `--dry-run` (single-process `run`) has no distributed-mode equivalent
  yet.
