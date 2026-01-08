package scenario

import "testing"

func validScenario() Scenario {
	return Scenario{
		Version: "0.1",
		Info:    Info{Name: "smoke"},
		Target:  Target{RPCURL: "http://127.0.0.1:8545", ChainID: 31337, Environment: "local"},
		Load:    Load{Type: "constant", VUs: 1, Duration: Duration(1)},
		Wallets: Wallets{Count: 1},
		Steps:   []Step{{Action: "get_balance"}},
	}
}

func TestValidate_AcceptsValidScenario(t *testing.T) {
	s := validScenario()
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid scenario to pass, got: %v", err)
	}
}

func TestValidate_RejectsUnsupportedLoadType(t *testing.T) {
	s := validScenario()
	s.Load.Type = "arrival-rate"
	if err := s.Validate(); err == nil {
		t.Fatal("expected arrival-rate load type to be rejected in v0.1")
	}
}

func TestValidate_AcceptsSpikeLoad(t *testing.T) {
	s := validScenario()
	s.Load = Load{
		Type:          "spike",
		Baseline:      10,
		Target:        200,
		Before:        Duration(1),
		SpikeDuration: Duration(1),
		After:         Duration(1),
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid spike load to pass, got: %v", err)
	}
}

func TestValidate_RejectsSpikeWithoutRise(t *testing.T) {
	s := validScenario()
	s.Load = Load{Type: "spike", Baseline: 100, Target: 100, Before: Duration(1), SpikeDuration: Duration(1), After: Duration(1)}
	if err := s.Validate(); err == nil {
		t.Fatal("expected spike with target <= baseline to be rejected")
	}
}

func TestValidate_AcceptsStressLoad(t *testing.T) {
	s := validScenario()
	s.Load = Load{Type: "stress", Start: 10, Step: 10, StageDuration: Duration(1), Max: 30}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid stress load to pass, got: %v", err)
	}
}

func TestValidate_AcceptsSoakLoad(t *testing.T) {
	s := validScenario()
	s.Load = Load{Type: "soak", VUs: 5, Duration: Duration(1)}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid soak load to pass, got: %v", err)
	}
}

func TestValidate_RejectsUnknownAction(t *testing.T) {
	s := validScenario()
	s.Steps = []Step{{Action: "flash_loan"}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected unknown action to be rejected")
	}
}

func TestValidate_RejectsPrivateKeyVariable(t *testing.T) {
	s := validScenario()
	s.Vars = map[string]string{"private_key": "0xdeadbeef"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected a 'private_key' variable to be rejected")
	}
}

func TestValidate_RejectsContractCallMissingFields(t *testing.T) {
	s := validScenario()
	s.Steps = []Step{{Action: "contract_call", Contract: "0x1"}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected contract_call without method/abi_file to be rejected")
	}
}

func TestValidate_RejectsMissingChainID(t *testing.T) {
	s := validScenario()
	s.Target.ChainID = 0
	if err := s.Validate(); err == nil {
		t.Fatal("expected missing chain_id to be rejected")
	}
}
