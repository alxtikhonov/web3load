package main

import (
	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/report"
)

func newReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report <results.json>",
		Short: "Re-render a saved JSON result as a console report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := report.ReadJSON(args[0])
			if err != nil {
				return err
			}
			report.WriteConsole(cmd.OutOrStdout(), r)
			return nil
		},
	}
}
