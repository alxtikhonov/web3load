package scenario

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestScenario_YAMLRoundTrip verifies a Scenario can be marshaled back to
// YAML and re-parsed identically — what the distributed controller relies
// on to ship a scenario (including a sharded Load) to a worker as text.
// Duration's custom MarshalYAML is the part most likely to silently break
// this if it's ever removed.
func TestScenario_YAMLRoundTrip(t *testing.T) {
	original := Scenario{
		Version: "0.1",
		Info:    Info{Name: "roundtrip_test"},
		Target:  Target{RPCURL: "http://127.0.0.1:8545", ChainID: 31337, Environment: "local"},
		Load: Load{
			Type:   "ramping",
			Stages: []Stage{{Duration: Duration(2 * 60 * 1e9), Target: 100}, {Duration: Duration(500 * 1e6), Target: 0}},
		},
		Wallets: Wallets{Count: 100},
		Vars:    map[string]string{"recipient": "0xdead"},
		Steps: []Step{
			{Action: "transfer", To: "${recipient}", Amount: "1000", WaitForConfirmation: true, Think: Duration(1e9)},
		},
		Asserts: []string{"transaction_success_rate > 99%"},
		Safety:  Safety{MaxGasPriceGwei: 50, AllowedChainIDs: []int64{31337}},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML on marshaled output: %v\n---\n%s", err, data)
	}

	if parsed.Info.Name != original.Info.Name {
		t.Errorf("name: got %q, want %q", parsed.Info.Name, original.Info.Name)
	}
	if parsed.Load.Type != original.Load.Type || len(parsed.Load.Stages) != len(original.Load.Stages) {
		t.Fatalf("load mismatch: got %+v, want %+v", parsed.Load, original.Load)
	}
	for i := range original.Load.Stages {
		if parsed.Load.Stages[i].Duration.AsTime() != original.Load.Stages[i].Duration.AsTime() {
			t.Errorf("stage %d duration: got %s, want %s", i, parsed.Load.Stages[i].Duration.AsTime(), original.Load.Stages[i].Duration.AsTime())
		}
		if parsed.Load.Stages[i].Target != original.Load.Stages[i].Target {
			t.Errorf("stage %d target: got %d, want %d", i, parsed.Load.Stages[i].Target, original.Load.Stages[i].Target)
		}
	}
	if parsed.Steps[0].Think.AsTime() != original.Steps[0].Think.AsTime() {
		t.Errorf("think duration: got %s, want %s", parsed.Steps[0].Think.AsTime(), original.Steps[0].Think.AsTime())
	}
	if parsed.Safety.MaxGasPriceGwei != original.Safety.MaxGasPriceGwei {
		t.Errorf("max_gas_price_gwei: got %v, want %v", parsed.Safety.MaxGasPriceGwei, original.Safety.MaxGasPriceGwei)
	}
}
