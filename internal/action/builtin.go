package action

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/txengine"
)

func init() {
	Register("get_balance", func() Action { return &getBalance{} })
	Register("transfer", func() Action { return &transfer{} })
	Register("erc20_transfer", func() Action { return &erc20Transfer{} })
	Register("approve", func() Action { return &approve{} })
	Register("contract_call", func() Action { return &contractCall{} })
}

var erc20ABI = mustParseABI(`[
  {"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"},
  {"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"}
]`)

func mustParseABI(js string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(js))
	if err != nil {
		panic(fmt.Sprintf("action: invalid built-in ABI: %v", err))
	}
	return parsed
}

// --- get_balance: read-only, no transaction engine involved. ---

type getBalance struct{}

func (a *getBalance) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	bal, err := deps.Adapter.BalanceAt(ctx, state.Wallet.Address)
	if err != nil {
		return Result{}, fmt.Errorf("get_balance: %w", err)
	}
	return Result{Success: true, Value: bal.String()}, nil
}

// --- transfer: native currency. ---

type transfer struct{}

func (a *transfer) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	amount, ok := new(big.Int).SetString(step.Amount, 10)
	if !ok {
		return Result{}, fmt.Errorf("transfer: invalid amount %q", step.Amount)
	}
	if err := checkSpendCap(deps.Safety, amount); err != nil {
		return Result{}, err
	}
	to := resolveAddress(step.To, state)
	out, err := deps.Engine.Send(ctx, state.Wallet, &to, amount, nil, 21000, step.WaitForConfirmation)
	return outcomeToResult(out, err)
}

// --- erc20_transfer ---

type erc20Transfer struct{}

func (a *erc20Transfer) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	token := resolveAddress(step.Token, state)
	to := resolveAddress(step.To, state)
	amount, ok := new(big.Int).SetString(step.Amount, 10)
	if !ok {
		return Result{}, fmt.Errorf("erc20_transfer: invalid amount %q", step.Amount)
	}
	data, err := erc20ABI.Pack("transfer", to, amount)
	if err != nil {
		return Result{}, fmt.Errorf("erc20_transfer: encode: %w", err)
	}
	out, err := deps.Engine.Send(ctx, state.Wallet, &token, big.NewInt(0), data, 0, step.WaitForConfirmation)
	return outcomeToResult(out, err)
}

// --- approve ---

type approve struct{}

func (a *approve) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	token := resolveAddress(step.Token, state)
	spender := resolveAddress(step.Spender, state)
	amount, ok := new(big.Int).SetString(step.Amount, 10)
	if !ok {
		return Result{}, fmt.Errorf("approve: invalid amount %q", step.Amount)
	}
	data, err := erc20ABI.Pack("approve", spender, amount)
	if err != nil {
		return Result{}, fmt.Errorf("approve: encode: %w", err)
	}
	out, err := deps.Engine.Send(ctx, state.Wallet, &token, big.NewInt(0), data, 0, step.WaitForConfirmation)
	return outcomeToResult(out, err)
}

// --- contract_call: the generic escape hatch. Any ABI method on any
// contract can be driven from a scenario without a core code change — this
// is what keeps swap/mint/bridge-style operations out of the engine, and is
// the actual answer to "how do I add a new operation". ---

type contractCall struct{}

func (a *contractCall) Execute(ctx context.Context, deps Deps, step scenario.Step, state *VUState) (Result, error) {
	raw, err := os.ReadFile(step.ABIFile)
	if err != nil {
		return Result{}, fmt.Errorf("contract_call: read abi_file %s: %w", step.ABIFile, err)
	}
	parsed, err := abi.JSON(strings.NewReader(string(raw)))
	if err != nil {
		return Result{}, fmt.Errorf("contract_call: parse abi_file %s: %w", step.ABIFile, err)
	}
	method, ok := parsed.Methods[step.Method]
	if !ok {
		return Result{}, fmt.Errorf("contract_call: method %q not found in %s", step.Method, step.ABIFile)
	}

	resolvedArgs := make([]interface{}, len(step.Args))
	for i, arg := range step.Args {
		if s, isStr := arg.(string); isStr {
			resolvedArgs[i] = resolve(s, state)
			continue
		}
		if items, isList := arg.([]interface{}); isList {
			resolvedArgs[i] = resolveList(items, state)
			continue
		}
		resolvedArgs[i] = arg
	}

	args, err := coerceArgs(method.Inputs, resolvedArgs)
	if err != nil {
		return Result{}, fmt.Errorf("contract_call: %w", err)
	}
	data, err := parsed.Pack(step.Method, args...)
	if err != nil {
		return Result{}, fmt.Errorf("contract_call: encode %s: %w", step.Method, err)
	}

	contract := resolveAddress(step.Contract, state)
	out, err := deps.Engine.Send(ctx, state.Wallet, &contract, big.NewInt(0), data, 0, step.WaitForConfirmation)
	return outcomeToResult(out, err)
}

func resolveList(items []interface{}, state *VUState) []interface{} {
	out := make([]interface{}, len(items))
	for i, it := range items {
		if s, ok := it.(string); ok {
			out[i] = resolve(s, state)
		} else {
			out[i] = it
		}
	}
	return out
}

// --- shared helpers ---

func checkSpendCap(safety scenario.Safety, amount *big.Int) error {
	if safety.MaxSpendPerWalletNative == "" {
		return nil
	}
	maxAmt, ok := new(big.Int).SetString(safety.MaxSpendPerWalletNative, 10)
	if !ok {
		return nil
	}
	if amount.Cmp(maxAmt) > 0 {
		return fmt.Errorf("safety: transfer amount %s exceeds max_spend_per_wallet_native %s", amount, maxAmt)
	}
	return nil
}

func outcomeToResult(out txengine.Outcome, err error) (Result, error) {
	res := Result{
		TxHash:         out.Hash.Hex(),
		GasUsed:        out.GasUsed,
		SubmitLatency:  out.SubmitLatency,
		ConfirmLatency: out.ConfirmLatency,
	}
	if err != nil {
		return res, err
	}
	if out.Waited {
		res.Success = out.Success
		if out.Reverted {
			res.RevertReason = "reverted"
		}
	} else {
		res.Success = true
	}
	return res, nil
}
