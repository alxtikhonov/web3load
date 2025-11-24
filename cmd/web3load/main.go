// Command web3load is the CLI entrypoint: validate, run, wallets
// generate/fund, and report.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "web3load",
		Short: "Load testing for Web3 infrastructure — RPC, transactions, and smart contract flows.",
	}
	root.AddCommand(newValidateCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newWalletsCmd())
	root.AddCommand(newReportCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
