// Package action implements the built-in step types a scenario can use, and
// the registry that maps a YAML `action:` name to an implementation. New
// operations plug in here without the load engine or scenario package ever
// needing to know about them — see CONTRIBUTING.md.
package action

import (
	"context"
	"time"

	"github.com/web3load/web3load/internal/chain"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/txengine"
	"github.com/web3load/web3load/internal/wallet"
)

// VUState is the per-virtual-user context a step executes against.
type VUState struct {
	Wallet  wallet.Wallet
	Vars    map[string]string
	Saved   map[string]interface{}
	ChainID int64
}

type Result struct {
	Success        bool
	TxHash         string
	GasUsed        uint64
	RevertReason   string
	SubmitLatency  time.Duration // 0 for read-only actions like get_balance
	ConfirmLatency time.Duration // 0 unless wait_for_confirmation was set
	Value          interface{}   // saved under step.save_as when non-nil
}

// Deps are the shared, run-scoped dependencies every action needs. They're
// constructed once per `run` invocation and passed to every step of every
// virtual user.
type Deps struct {
	Adapter chain.Adapter
	Engine  *txengine.Engine
	Safety  scenario.Safety
}

type Action interface {
	Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error)
}

type Factory func() Action

var registry = map[string]Factory{}

func Register(name string, f Factory) {
	registry[name] = f
}

func Get(name string) (Action, bool) {
	f, ok := registry[name]
	if !ok {
		return nil, false
	}
	return f(), true
}
