# Scenario DSL reference (v0.1)

## Top-level structure

```yaml
version: "0.1"

target:
  rpc_url: "http://127.0.0.1:8545"   # required
  chain_id: 31337                    # required; run refuses to start if the RPC reports a different chain id
  environment: local                 # required: local | testnet | private | production

scenario:
  name: my_scenario                  # required

load:
  # see "Load models" below

wallets:
  count: 1000                        # required, > 0
  fund:                              # optional; documents funding intent for the
    native: "1000000000000000000"    # scenario. NOT YET consumed by the CLI in v0.1 —
    from: ${FUNDER_PRIVATE_KEY}       # `web3load wallets fund` takes --from/--native
                                      # flags directly. Wiring this block up so `run`
                                      # can fund automatically is a v0.2 item.

variables:
  usdc: "0xA0b8...eB48"               # arbitrary string variables, referenced as ${usdc}

steps:
  - action: ...

assertions:
  - transaction_success_rate > 99%

safety:
  max_gas_price_gwei: 100
  max_spend_per_wallet_native: "1000000000000000000"
  allowed_chain_ids: [31337, 11155111]
```

Unknown top-level or step fields are a parse error (strict YAML decoding) —
this is deliberate: it's also what stops an inline `private_key:` field from
being silently accepted anywhere in a scenario.

## Load models (v0.1)

Only `constant` and `ramping` ship in v0.1. `spike`, `stress`, `soak`, and
`arrival-rate` are v0.2 roadmap items — they're parameterizations of the same
scheduler, not a different engine.

```yaml
load:
  type: constant
  vus: 50
  duration: 2m
```

```yaml
load:
  type: ramping
  stages:
    - { duration: 2m, target: 10 }
    - { duration: 6m, target: 1000 }
    - { duration: 2m, target: 0 }
```

Each virtual user is assigned a wallet round-robin from the keystore and
repeats `steps` in a loop for as long as it's kept alive. `wallets.count`
should generally be >= the peak `target` so no two concurrently active VUs
ever share a wallet's in-flight nonce.

## Variable substitution

`${name}` is replaced by, in order:

- `wallet.address` — the acting VU's own address
- a key from `variables:`
- a name previously written via a step's `save_as:`

An unresolved `${...}` is left verbatim (so a typo fails loudly downstream
rather than silently sending to the zero address).

Relative-time expressions like `${now + 300}` (useful for swap deadlines)
are **not** supported in v0.1 — use a literal future unix timestamp. This is
a documented roadmap gap, not an oversight.

## Steps

Every step is either an `action` or a `think` pause:

```yaml
- think: 500ms
```

### `get_balance`

```yaml
- action: get_balance
  save_as: balance_before   # optional; makes the value available as ${balance_before}
```

### `transfer` (native currency)

```yaml
- action: transfer
  to: ${recipient}
  amount: "1000000000000000"   # wei, as a decimal string (avoid float precision loss)
  wait_for_confirmation: true  # default false: fire-and-forget, only submission is measured
```

### `erc20_transfer`

```yaml
- action: erc20_transfer
  token: ${usdc}
  to: ${recipient}
  amount: "1000000"
  wait_for_confirmation: true
```

### `approve`

```yaml
- action: approve
  token: ${usdc}
  spender: ${router}
  amount: "1000000000"
  wait_for_confirmation: true
```

### `contract_call` — the generic escape hatch

Any ABI method on any contract, without a core code change:

```yaml
- action: contract_call
  contract: ${router}
  abi_file: examples/abis/UniswapV2Router.json
  method: swapExactTokensForTokens
  args: [100000000, 0, ["${usdc}", "${weth}"], "${wallet.address}", 1893456000]
  wait_for_confirmation: true
```

Argument coercion follows the ABI's declared types: `uint*/int*` accept a
YAML number or a quoted decimal string (prefer quoted strings for anything
that might exceed 2^63), `address` expects a hex string, `bool` a YAML
boolean, `bytes`/`bytesN` a `0x`-prefixed hex string, and arrays/slices a
YAML list of the element type. Tuples/structs are not supported in v0.1.

## `wait_for_confirmation`

- `false` (default): the step is measured as submission latency only
  (`rpc_p*` assertions). Fast, but you learn nothing about whether the
  transaction actually landed.
- `true`: the engine also polls for the receipt, giving you confirmation
  latency (`confirmation_p*`), gas used, and revert detection — at the cost
  of holding the VU's goroutine open until it's mined or times out (60s
  default).

## Assertions

```yaml
assertions:
  - transaction_success_rate > 99%
  - rpc_p95 < 500ms
  - confirmation_p95 < 30s
  - gas_used_p95 < 250000
  - reverted_transactions < 10
  - nonce_errors == 0
  - rpc_errors < 5
```

Grammar: `<metric> <op> <value><unit?>`, `op` in `> < >= <= ==`, `unit` in
`% ms s` (omit for gas/counts). `web3load run` exits non-zero if any
assertion fails, after printing PASS/FAIL for each.

## Safety block

See [docs/security.md](docs/security.md).
