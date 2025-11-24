package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/chain/evm"
	"github.com/web3load/web3load/internal/load"
	"github.com/web3load/web3load/internal/metrics"
	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/txengine"
	"github.com/web3load/web3load/internal/wallet"
)

func newRunCmd() *cobra.Command {
	var walletsPath, out, metricsAddr string
	var dryRun bool
	c := &cobra.Command{
		Use:   "run <scenario.yaml>",
		Short: "Execute a scenario against a live EVM RPC endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := scenario.ParseFile(args[0])
			if err != nil {
				return err
			}

			if s.Target.Environment == "production" {
				if err := confirmProduction(cmd, s.Info.Name); err != nil {
					return err
				}
			}

			ks, err := wallet.Load(walletsPath)
			if err != nil {
				return fmt.Errorf("run: %w (generate one with 'web3load wallets generate')", err)
			}
			if len(ks.Wallets) < s.Wallets.Count {
				return fmt.Errorf("run: scenario requires %d wallets, keystore %s has %d", s.Wallets.Count, walletsPath, len(ks.Wallets))
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			adapter, err := evm.Dial(ctx, s.Target.RPCURL, s.Target.ChainID)
			if err != nil {
				return err
			}
			if len(s.Safety.AllowedChainIDs) > 0 && !containsChainID(s.Safety.AllowedChainIDs, s.Target.ChainID) {
				return fmt.Errorf("run: chain id %d is not in safety.allowed_chain_ids", s.Target.ChainID)
			}

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "dry-run: v0.1 dry-run validates and estimates gas only; use 'web3load validate' beforehand for schema checks. Full broadcast-skipping enforcement is a v0.2 item (see docs/security.md).")
			}

			nonces := wallet.NewNonceManager(adapter)
			engine := txengine.New(adapter, nonces)
			deps := action.Deps{Adapter: adapter, Engine: engine, Safety: s.Safety}

			collector := metrics.New()
			loadEngine := load.New(s, ks.Wallets[:s.Wallets.Count], deps, collector)

			if metricsAddr != "" {
				exporter := metrics.NewPrometheusExporter(collector, s.Info.Name)
				metricsCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				go func() {
					if err := exporter.Serve(metricsCtx, metricsAddr, time.Second); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "metrics server:", err)
					}
				}()
				fmt.Fprintf(cmd.OutOrStdout(), "prometheus metrics at http://%s/metrics\n", metricsAddr)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "running %q against %s (chain id %d)...\n", s.Info.Name, s.Target.RPCURL, s.Target.ChainID)
			if err := loadEngine.Run(ctx); err != nil {
				return err
			}

			result := collector.Snapshot(s.Info.Name)
			fmt.Fprintln(cmd.OutOrStdout())
			report.WriteConsole(cmd.OutOrStdout(), result)

			if out != "" {
				if err := report.WriteJSON(out, result); err != nil {
					return err
				}
			}
			return checkAssertions(cmd, s.Asserts, result)
		},
	}
	c.Flags().StringVar(&walletsPath, "wallets", "wallets.json", "keystore file to run the scenario against")
	c.Flags().StringVar(&out, "out", "", "write JSON results to this path")
	c.Flags().StringVar(&metricsAddr, "metrics-addr", "", "expose Prometheus metrics at this address (e.g. :9090) while running")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "estimate gas without broadcasting (partial in v0.1, see docs/security.md)")
	return c
}

func confirmProduction(cmd *cobra.Command, scenarioName string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "target.environment is 'production' — this will send real transactions on a live network.")
	fmt.Fprintf(cmd.OutOrStdout(), "Type the scenario name (%q) to confirm: ", scenarioName)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if trimNewline(line) != scenarioName {
		return fmt.Errorf("run: confirmation did not match scenario name, aborting")
	}
	return nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func containsChainID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func checkAssertions(cmd *cobra.Command, exprs []string, result report.Result) error {
	if len(exprs) == 0 {
		return nil
	}
	results, err := report.EvaluateAssertions(exprs, result)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout())
	var failed int
	for _, a := range results {
		status := "PASS"
		if !a.Passed {
			status = "FAIL"
			failed++
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", status, a.Expr)
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d assertions failed", failed, len(results))
	}
	return nil
}
