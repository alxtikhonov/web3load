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
	s.Load.Type = "spike"
	if err := s.Validate(); err == nil {
		t.Fatal("expected spike load type to be rejected in v0.1")
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
