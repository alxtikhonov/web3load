package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/scenario"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <scenario.yaml>",
		Short: "Parse and validate a scenario file without connecting to any chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := scenario.ParseFile(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: %q is valid (%s load, %d wallets, %d steps)\n",
				s.Info.Name, s.Load.Type, s.Wallets.Count, len(s.Steps))
			return nil
		},
	}
}
