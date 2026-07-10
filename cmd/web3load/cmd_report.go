package main

import (
	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/report"
)

func newReportCmd() *cobra.Command {
	var htmlOut string
	c := &cobra.Command{
		Use:   "report <results.json>",
		Short: "Re-render a saved JSON result as a console report (and optionally HTML)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := report.ReadJSON(args[0])
			if err != nil {
				return err
			}
			report.WriteConsole(cmd.OutOrStdout(), r)
			if htmlOut != "" {
				// The saved JSON doesn't carry assertion outcomes (those are
				// a `run`-time concern, not part of report.Result), so a
				// report regenerated this way has no assertions section.
				if err := report.WriteHTML(htmlOut, r, nil); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&htmlOut, "html", "", "also write a self-contained HTML report to this path")
	return c
}
