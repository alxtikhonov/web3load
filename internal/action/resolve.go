package action

import (
	"fmt"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
)

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// resolve expands ${name} placeholders against, in order: the acting
// wallet's own address (${wallet.address}), scenario variables, and values
// saved by earlier steps via save_as. An unresolved placeholder is left
// verbatim so a typo fails loudly downstream (e.g. as an invalid address)
// instead of silently resolving to the zero value.
func resolve(raw string, state *VUState) string {
	return varPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := match[2 : len(match)-1]
		if name == "wallet.address" {
			return state.Wallet.Address.Hex()
		}
		if v, ok := state.Vars[name]; ok {
			return v
		}
		if v, ok := state.Saved[name]; ok {
			return fmt.Sprint(v)
		}
		return match
	})
}

func resolveAddress(raw string, state *VUState) common.Address {
	return common.HexToAddress(resolve(raw, state))
}
