# Web3Load

[![CI](https://github.com/alxtikhonov/web3load/actions/workflows/ci.yml/badge.svg)](https://github.com/alxtikhonov/web3load/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/alxtikhonov/web3load)](https://goreportcard.com/report/github.com/alxtikhonov/web3load)
[![Go Reference](https://pkg.go.dev/badge/github.com/alxtikhonov/web3load.svg)](https://pkg.go.dev/github.com/alxtikhonov/web3load)
![Go version](https://img.shields.io/github/go-mod/go-version/alxtikhonov/web3load)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Status](https://img.shields.io/badge/status-v0.2-blue)

**Load testing for blockchain infrastructure — the way k6 does it for APIs.**

Web3Load is an open-source, single-binary load testing framework built for
Web3: RPC nodes, smart contracts, and real user flows like swaps, mints, and
transfers — not generic HTTP requests wearing a blockchain costume.

Wallets, nonces, gas, and transaction confirmation are first-class concepts
in the scenario language and the execution engine, not something you script
by hand on top of ethers.js/web3.py every time.

## Why not k6 / Locust / JMeter?

They treat an RPC call as just another HTTP request. Web3Load knows the
difference between a transaction being *submitted* and being *confirmed*,
manages nonces safely across thousands of concurrent wallets without
collisions, and reports gas economics and revert rates as first-class
metrics — not something you bolt on yourself in every project.

See [docs/architecture](docs/) for the full design rationale.

## Status

**v0.2** — EVM only, single-process, tested against
[Anvil](https://book.getfoundry.sh/anvil/). All six load models
(constant/ramping/spike/stress/soak/arrival-rate), encrypted keystores,
structured logs, OpenTelemetry tracing, and a Grafana dashboard. See the
roadmap below for what's next (v0.3: distributed mode, plugins, Solana).

## Quickstart

```bash
# 1. local EVM node
anvil

# 2. generate + fund test wallets
go run ./cmd/web3load wallets generate --count 20 --out wallets.json
go run ./cmd/web3load wallets fund \
  --wallets wallets.json \
  --from 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 \
  --native 1000000000000000000

# 3. validate and run a scenario
go run ./cmd/web3load validate examples/native_transfer.yaml
go run ./cmd/web3load run examples/native_transfer.yaml \
  --wallets wallets.json \
  --out results.json

# 4. re-render a saved result
go run ./cmd/web3load report results.json
```

`--from` above is Anvil's well-known default account #0 — never use it, or
any key committed to a repo, on a network holding real funds.

## Declarative scenarios

```yaml
load:
  type: ramping
  stages:
    - { duration: 2m, target: 10 }
    - { duration: 6m, target: 1000 }

steps:
  - action: approve
    token: ${usdc}
    spender: ${router}
    amount: "1000000000"

  - action: contract_call
    contract: ${router}
    abi_file: abis/UniswapV2Router.json
    method: swapExactTokensForTokens
    args: [100000000, 0, ["${usdc}", "${weth}"], "${wallet.address}", 1893456000]

assertions:
  - transaction_success_rate > 99%
  - rpc_p95 < 500ms
  - confirmation_p95 < 30s
```

`contract_call` is the general escape hatch: any ABI method on any contract
can be driven from a scenario without a core code change. `approve` and
`transfer` are just pre-baked conveniences over the same mechanism. See
[docs/dsl-reference.md](docs/dsl-reference.md) for the full schema.

## What's in

- `web3load validate` / `run` / `wallets generate` / `wallets fund` / `report`
- Load models: `constant`, `ramping`, `spike`, `stress`, `soak`, `arrival-rate` — see [docs/dsl-reference.md](docs/dsl-reference.md#load-models)
- Actions: `get_balance`, `transfer`, `erc20_transfer`, `approve`, `contract_call`, each retryable via a per-step `retry` policy that's safe against double-broadcast
- EVM chain adapter (works against any EVM-compatible RPC, tested on Anvil)
- Nonce management safe under concurrent virtual users, with automatic resync on a nonce mismatch
- Plaintext or encrypted (scrypt + AES-256-GCM) keystores
- Console + JSON reports, scenario assertions with pass/fail exit code
- Structured logs (`--log-level`, `--log-format`) and periodic progress snapshots (`--progress-interval`) — see [docs/observability.md](docs/observability.md)
- Prometheus `/metrics` endpoint with an auto-provisioned Grafana dashboard ([deploy/grafana](deploy/grafana)), and OpenTelemetry trace export (`--otel-endpoint`)

Not yet: distributed load generation, dynamic plugins, non-EVM chains. See
the roadmap.

## Roadmap

| Version | Focus |
|---|---|
| v0.1 | MVP: constant/ramping load, EVM, wallet+nonce management, core actions |
| v0.2 | ✅ all six load models, encrypted keystores, retry policies, structured logs, OpenTelemetry tracing, Grafana dashboard |
| v0.3 | Distributed mode (controller/worker), plugin system, experimental Solana adapter |
| v1.0 | Stable DSL/schema, Solana GA, adapter conformance suite, docs site |

## Security

Private keys never appear in logs or scenario files — only keystore
references. Keystores are plaintext by default; pass `--password` (or set
`WEB3LOAD_KEYSTORE_PASSWORD`) to `wallets generate` to encrypt at rest
(scrypt + AES-256-GCM). Either way, treat `wallets.json` like a secret.
`production` environments require an explicit typed confirmation before any
transaction is broadcast. See [docs/security.md](docs/security.md).

## Contributing

New chains and actions plug in through the `chain.Adapter` and
`action.Action` interfaces — no core engine changes required. See
[docs/adapters.md](docs/adapters.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
