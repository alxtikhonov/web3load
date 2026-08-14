package action

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/plugin"
	"github.com/web3load/web3load/internal/scenario"
)

// pluginAction bridges a running plugin subprocess into the action
// registry. The plugin itself never sees a private key: it either returns
// a transaction spec (to/value/data), which this host builds, signs, and
// submits via the normal txengine — nonce management, gas estimation, and
// the safety clamps all still apply exactly as they do to a built-in
// action — or a plain non-transactional result.
type pluginAction struct {
	name    string
	process *plugin.Process
}

// RegisterPlugin wires a running plugin subprocess as an action under
// name (conventionally "plugin:<name>" — see scenario.Step's validation),
// so scenario steps referencing it resolve through the normal action
// registry like any built-in.
func RegisterPlugin(name string, process *plugin.Process) {
	Register(name, func() Action { return &pluginAction{name: name, process: process} })
}

func (a *pluginAction) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	resolvedArgs := make([]interface{}, len(step.Args))
	for i, arg := range step.Args {
		switch v := arg.(type) {
		case string:
			resolvedArgs[i] = resolve(v, state)
		case []interface{}:
			resolvedArgs[i] = resolveList(v, state)
		default:
			resolvedArgs[i] = v
		}
	}

	resp, err := a.process.Call(plugin.Request{
		Method:   step.Method,
		Contract: resolve(step.Contract, state),
		Args:     resolvedArgs,
		Vars:     state.Vars,
		Wallet:   state.Wallet.Address.Hex(),
		Saved:    state.Saved,
	})
	if err != nil {
		return Result{}, fmt.Errorf("plugin %s: %w", a.name, err)
	}

	switch resp.Kind {
	case "error":
		return Result{}, fmt.Errorf("plugin %s: %s", a.name, resp.Error)

	case "result":
		var value interface{}
		if resp.Output != "" {
			value = resp.Output
		}
		return Result{Success: resp.Success, Value: value}, nil

	case "transaction":
		to := common.HexToAddress(resp.To)
		value := big.NewInt(0)
		if resp.Value != "" {
			v, ok := new(big.Int).SetString(resp.Value, 10)
			if !ok {
				return Result{}, fmt.Errorf("plugin %s: invalid value %q", a.name, resp.Value)
			}
			value = v
		}
		data, err := hexToBytes(resp.Data)
		if err != nil {
			return Result{}, fmt.Errorf("plugin %s: invalid data: %w", a.name, err)
		}
		out, err := deps.Engine.Send(ctx, state.Wallet, &to, value, data, resp.GasLimit, resp.WaitForConfirmation)
		return outcomeToResult(out, err)

	default:
		return Result{}, fmt.Errorf("plugin %s: unrecognized response kind %q", a.name, resp.Kind)
	}
}
