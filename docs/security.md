# Security model

## Private keys

- Never embedded in scenario YAML. The parser uses strict field decoding
  (`yaml.Decoder.KnownFields(true)`), so an unexpected field like an inline
  `private_key:` fails to parse instead of silently being accepted.
- Never logged: `wallet.Wallet` implements `String()` so the private key is
  redacted in any `%v`/`%s`/`%+v` format, including accidental
  `fmt.Println(wallet)` calls.
- **v0.1 keystores (`wallets.json`) are plaintext on disk.** `wallets
  generate` prints an explicit warning about this. Treat the file like a
  secret: don't commit it, don't reuse it against a chain holding real
  value. Encrypted keystores (scrypt + AES-GCM) are a v0.2 commitment, not a
  "maybe."
- RPC URLs/credentials are read from `${ENV_VAR}` references, never written
  into scenario files or logs in resolved form.

## Guarding against running on the wrong network

- `evm.Dial` fetches the RPC's actual `eth_chainId` and **refuses to
  proceed** if it doesn't match `target.chain_id` in the scenario. This is
  the single most common way a load test accidentally lands on the wrong
  network — a copy-pasted scenario pointed at a stale `RPC_URL` env var.
- `target.environment: production` requires typing the scenario's exact
  name back at a confirmation prompt before `run` broadcasts anything.
- `safety.allowed_chain_ids` is an explicit allow-list; if set, `run`
  refuses to start against any chain id not on it.

## Guarding against accidental spend

- `safety.max_spend_per_wallet_native` bounds any single native-currency
  transfer; `transfer` rejects amounts above it before signing.
- `safety.max_gas_price_gwei` is intended to bound `SuggestFees` (full
  enforcement — clamping rather than just rejecting — is a v0.2 item;
  tracked so it doesn't get lost).
- `web3load validate` never broadcasts anything — it only parses and checks
  the schema. `run --dry-run` executes the full scenario logic but skips
  `eth_sendRawTransaction` (gas estimation and validation still happen).

## Rate limiting

`internal/rpc` is the intended home for a token-bucket limiter around the
adapter's RPC calls, to avoid a runaway scenario getting an API key banned
by a third-party provider (Infura/Alchemy/etc.). v0.1 ships basic
retry-with-backoff (`internal/rpc/retry.go`); the limiter itself is a v0.2
item — flagged here so it isn't dropped from scope.

## Threat model explicitly out of scope for v0.1

- Multi-tenant / untrusted scenario authors (the tool assumes whoever runs
  `web3load run` is trusted with the keys they point it at).
- Protection against a malicious `abi_file` or ABI-encoded call itself
  doing something harmful on-chain — that's a property of the contract and
  the operator's judgment, not something the load tool can validate.
