# Contributing to Web3Load

## Setup

```bash
go mod tidy
make build
make test
```

## Adding a new action

Actions are the extension point for smart-contract operations (mint, bridge,
stake, ...). You do **not** need to touch the core engine:

1. Implement the `action.Action` interface (`internal/action/action.go`).
2. Register it in `init()`, e.g. `action.Register("my_action", func() action.Action { return &myAction{} })`.
3. Add it to the allow-list in `scenario.builtinActions` (`internal/scenario/validate.go`)
   so scenarios referencing it pass validation.
4. Add an example scenario under `examples/`.

If your operation doesn't need a bespoke Go type at all, prefer documenting
it as a `contract_call` example instead — that's the generic path and keeps
the action registry small.

## Adding a new chain

Implement `chain.Adapter` (`internal/chain/adapter.go`) in a new
`internal/chain/<name>` package. Nothing outside that package should need to
change. See [docs/adapters.md](docs/adapters.md).

## Pull requests

- Keep PRs scoped to one component where possible (easier to review against
  the architecture doc).
- Add a test for new logic in `internal/*`; CLI wiring in `cmd/web3load` can
  be covered by an example scenario + manual run against Anvil.
- `go vet ./...` and `go test ./... -race` must pass.
