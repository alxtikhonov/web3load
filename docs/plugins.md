# Plugins

A plugin adds a new step type without touching Web3Load's core — a
separate program, launched as a subprocess, that computes what a step
should do. Two working examples ship in `examples/plugins/`.

## Quickstart

```bash
go build -o deadline ./examples/plugins/deadline
web3load run examples/plugin_deadline.yaml --plugin deadline=./deadline
```

In the scenario, a plugin step looks like any other, prefixed `plugin:`:

```yaml
steps:
  - action: plugin:deadline
    args: [300]
    save_as: swap_deadline
```

`--plugin <name>=<path>` (repeatable) starts `path` as a subprocess and
registers it under `plugin:<name>`. A `plugin:` action not backed by a
loaded plugin fails at run time with "unknown action" — `validate` accepts
the name unconditionally (it can't know in advance which plugins a given
`run` invocation will load) but can't guarantee it'll actually resolve.

Distributed mode: pass the same `--plugin` flags to every `worker` — each
runs its own local `load.Engine` and needs the plugin binary available on
that machine.

## Why subprocess + JSON on stdin/stdout, not gRPC

The architecture doc sketched `hashicorp/go-plugin`-style subprocess+gRPC.
v0.3 ships something simpler instead, for the same reason distributed mode
(docs/distributed.md) uses HTTP/JSON instead of gRPC: no `protoc` toolchain
dependency, and — for the classic non-gRPC mode of go-plugin — no `gob`
encoding of `interface{}`-typed fields to get right (`scenario.Step.Args`
and `action.Result.Value` are both `interface{}`-shaped, which `gob`
can silently mishandle without upfront type registration; JSON has no such
gotcha). The tradeoff is real: no streaming, no multiplexed connections,
one request in flight at a time per plugin process. For the actual
workload — one call per executed step, not a high-frequency data
plane — that's a fine trade, not a shortcut that will need revisiting soon.

The upside this choice keeps is the one that mattered most: **a plugin can
be written in any language that can read a line from stdin and write a
line to stdout.** No Web3Load SDK, no code generation, no Go dependency at
all — both example plugins are plain Go programs that import nothing from
this repository.

## Protocol

One request, one response, each a single line of JSON.

**Request** (host → plugin, on stdin):

```json
{"method": "swap", "contract": "0x...", "args": [...], "vars": {...}, "wallet": "0x...", "saved": {...}}
```

`wallet` is an **address only** — plugins never receive private key
material. `contract` and any string inside `args` have already had
`${...}` scenario variables resolved by the host before the plugin sees
them (the same substitution `contract_call` uses).

**Response** (plugin → host, on stdout), one of three shapes selected by
`kind`:

```json
{"kind": "transaction", "to": "0x...", "value": "1000000000000000", "data": "0x...", "gas_limit": 0, "wait_for_confirmation": true}
```

The plugin computed what to send; the host builds, signs (with the real
private key, which never left the host process), and submits it through
the same `txengine.Engine` a built-in action uses — nonce management, gas
estimation, and the `safety.max_gas_price_gwei` clamp all apply exactly as
they do anywhere else. `gas_limit: 0` means "let the host estimate it."

```json
{"kind": "result", "success": true, "output": "1893456000"}
```

A non-transactional outcome — no chain interaction at all for this step.
`output`, if non-empty, becomes the step's `save_as` value (as a string).
`examples/plugins/deadline` is exactly this: it computes a future Unix
timestamp and returns it as `output`, filling the one documented DSL gap
(relative-time expressions aren't supported in YAML directly — see
docs/dsl-reference.md) through the extension mechanism instead of a core
change.

```json
{"kind": "error", "error": "human-readable explanation"}
```

The step fails with this message.

## Current limits

- **One subprocess per plugin, not a pool.** Every VU using
  `plugin:foo` shares the same process and its calls are serialized
  through a mutex (`internal/plugin.Process.Call`) — correct, but a plugin
  is a throughput ceiling for any scenario pushing serious concurrency
  through it. Sized right for "a plugin computes something a bit more
  involved than `contract_call` can express," not for "a plugin *is* the
  hot path of a 1000-VU load test." A pool is a natural v1.0 extension if
  that turns out to matter in practice.
- **No plugin discovery or versioning** — you point `--plugin` at an exact
  path; there's no registry, no manifest, no compatibility check beyond
  "the JSON round-trips."
- **No streaming/partial responses** — one full request in, one full
  response out.
