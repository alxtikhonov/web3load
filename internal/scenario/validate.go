package scenario

import "fmt"

// builtinActions is the v0.1 allow-list. New actions registered in
// internal/action must be added here too, or scenarios referencing them
// fail validation before ever reaching the load engine.
var builtinActions = map[string]bool{
	"get_balance":    true,
	"transfer":       true,
	"erc20_transfer": true,
	"approve":        true,
	"contract_call":  true,
}

func (s *Scenario) Validate() error {
	if s.Version == "" {
		return fmt.Errorf("version is required")
	}
	if s.Info.Name == "" {
		return fmt.Errorf("scenario.name is required")
	}

	if err := s.Target.validate(); err != nil {
		return err
	}
	if err := s.Load.validate(); err != nil {
		return err
	}

	if s.Wallets.Count <= 0 {
		return fmt.Errorf("wallets.count must be > 0")
	}

	if len(s.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	for i, step := range s.Steps {
		if err := step.validate(i); err != nil {
			return err
		}
	}

	if _, reserved := s.Vars["private_key"]; reserved {
		return fmt.Errorf("variables: \"private_key\" is not allowed; private keys must come from the wallet keystore, never from scenario variables")
	}

	return nil
}

func (t Target) validate() error {
	if t.RPCURL == "" {
		return fmt.Errorf("target.rpc_url is required")
	}
	if t.ChainID == 0 {
		return fmt.Errorf("target.chain_id is required")
	}
	switch t.Environment {
	case "local", "testnet", "private", "production":
		return nil
	case "":
		return fmt.Errorf("target.environment is required (local|testnet|private|production)")
	default:
		return fmt.Errorf("target.environment: unknown value %q", t.Environment)
	}
}

func (l Load) validate() error {
	switch l.Type {
	case "constant", "soak":
		if l.VUs <= 0 {
			return fmt.Errorf("load.vus must be > 0 for %s load", l.Type)
		}
		if l.Duration.AsTime() <= 0 {
			return fmt.Errorf("load.duration must be > 0 for %s load", l.Type)
		}
	case "ramping":
		if len(l.Stages) == 0 {
			return fmt.Errorf("load.stages must have at least one stage for ramping load")
		}
		return validateStages(l)
	case "spike":
		if l.Target <= l.Baseline {
			return fmt.Errorf("load.target must be greater than load.baseline for spike load")
		}
		if l.Before.AsTime() <= 0 || l.SpikeDuration.AsTime() <= 0 || l.After.AsTime() <= 0 {
			return fmt.Errorf("load.before, load.spike_duration, and load.after must all be > 0 for spike load")
		}
		return validateStages(l)
	case "stress":
		if l.Step <= 0 {
			return fmt.Errorf("load.step must be > 0 for stress load")
		}
		if l.Max <= l.Start {
			return fmt.Errorf("load.max must be greater than load.start for stress load")
		}
		if l.StageDuration.AsTime() <= 0 {
			return fmt.Errorf("load.stage_duration must be > 0 for stress load")
		}
		return validateStages(l)
	case "":
		return fmt.Errorf("load.type is required (constant|ramping|spike|stress|soak)")
	default:
		return fmt.Errorf("load.type %q is not supported in v0.1 (arrival-rate is a roadmap item)", l.Type)
	}
	return nil
}

// validateStages resolves spike/stress/ramping into concrete stages and
// sanity-checks them, reusing the exact expansion the load engine will run
// so validate can never accept something run would reject.
func validateStages(l Load) error {
	stages, err := l.ResolvedStages()
	if err != nil {
		return err
	}
	for i, st := range stages {
		if st.Duration.AsTime() <= 0 {
			return fmt.Errorf("load: resolved stage %d has non-positive duration", i)
		}
		if st.Target < 0 {
			return fmt.Errorf("load: resolved stage %d has a negative target", i)
		}
	}
	return nil
}

func (step Step) validate(i int) error {
	if step.Action == "" {
		if step.Think.AsTime() <= 0 {
			return fmt.Errorf("steps[%d]: either action or think must be set", i)
		}
		return nil
	}
	if !builtinActions[step.Action] {
		return fmt.Errorf("steps[%d]: unknown action %q", i, step.Action)
	}
	if step.Action == "contract_call" && (step.Contract == "" || step.Method == "" || step.ABIFile == "") {
		return fmt.Errorf("steps[%d]: contract_call requires contract, abi_file, and method", i)
	}
	return nil
}
