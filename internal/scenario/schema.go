// Package scenario parses and validates the Web3Load DSL (docs/dsl-reference.md)
// into a typed Scenario the rest of the engine executes against.
package scenario

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration so scenario YAML can write "2m", "500ms"
// instead of nanosecond integers.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) AsTime() time.Duration { return time.Duration(d) }

type Scenario struct {
	Version string  `yaml:"version"`
	Target  Target  `yaml:"target"`
	Info    Info    `yaml:"scenario"`
	Load    Load    `yaml:"load"`
	Wallets Wallets `yaml:"wallets"`
	Vars    map[string]string `yaml:"variables"`
	Steps   []Step   `yaml:"steps"`
	Asserts []string `yaml:"assertions"`
	Safety  Safety   `yaml:"safety"`
}

type Info struct {
	Name string `yaml:"name"`
}

type Target struct {
	RPCURL      string `yaml:"rpc_url"`
	ChainID     int64  `yaml:"chain_id"`
	Environment string `yaml:"environment"` // local | testnet | private | production
}

type Load struct {
	Type     string   `yaml:"type"` // constant | ramping
	VUs      int      `yaml:"vus"`
	Duration Duration `yaml:"duration"`
	Stages   []Stage  `yaml:"stages"`
}

type Stage struct {
	Duration Duration `yaml:"duration"`
	Target   int      `yaml:"target"`
}

type Wallets struct {
	Count int       `yaml:"count"`
	Fund  *FundSpec `yaml:"fund"`
}

type FundSpec struct {
	Native string `yaml:"native"`
	From   string `yaml:"from"`
}

type Step struct {
	Action               string        `yaml:"action"`
	SaveAs               string        `yaml:"save_as"`
	Token                string        `yaml:"token"`
	Spender              string        `yaml:"spender"`
	To                   string        `yaml:"to"`
	Amount               string        `yaml:"amount"`
	Contract             string        `yaml:"contract"`
	ABIFile              string        `yaml:"abi_file"`
	Method               string        `yaml:"method"`
	Args                 []interface{} `yaml:"args"`
	WaitForConfirmation  bool          `yaml:"wait_for_confirmation"`
	Think                Duration      `yaml:"think"`
}

type Safety struct {
	MaxGasPriceGwei         float64 `yaml:"max_gas_price_gwei"`
	MaxSpendPerWalletNative string  `yaml:"max_spend_per_wallet_native"`
	AllowedChainIDs         []int64 `yaml:"allowed_chain_ids"`
}
