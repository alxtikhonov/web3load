package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/distributed"
	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/wallet"
)

func newControllerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "controller", Short: "Coordinate a scenario across multiple worker processes"}
	cmd.AddCommand(newControllerRunCmd())
	return cmd
}

func newControllerRunCmd() *cobra.Command {
	var walletsPath, listen, out, htmlOut, password string
	var numWorkers int
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "run <scenario.yaml>",
		Short: "Start a controller, wait for --workers worker processes to register, and aggregate their results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if numWorkers < 1 {
				return fmt.Errorf("controller: --workers must be >= 1")
			}
			s, err := scenario.ParseFile(args[0])
			if err != nil {
				return err
			}
			if s.Target.Environment == "production" {
				if err := confirmProduction(cmd, s.Info.Name); err != nil {
					return err
				}
			}

			ks, err := wallet.LoadAny(walletsPath, resolvePassword(password))
			if err != nil {
				return fmt.Errorf("controller: %w (generate one with 'web3load wallets generate')", err)
			}
			if len(ks.Wallets) < s.Wallets.Count {
				return fmt.Errorf("controller: scenario requires %d wallets, keystore %s has %d", s.Wallets.Count, walletsPath, len(ks.Wallets))
			}

			ctrl := distributed.NewController(s, ks.Wallets[:s.Wallets.Count], numWorkers)
			srv := &http.Server{Addr: listen, Handler: ctrl.Handler()}
			serveErr := make(chan error, 1)
			go func() { serveErr <- srv.ListenAndServe() }()

			fmt.Fprintf(cmd.OutOrStdout(), "controller listening on %s, waiting for %d worker(s) to register...\n", listen, numWorkers)
			fmt.Fprintf(cmd.OutOrStdout(), "start each with: web3load worker --controller http://<this-host>%s\n", listen)

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			waitCtx := ctx
			if timeout > 0 {
				var cancel context.CancelFunc
				waitCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			var waitErr error
			select {
			case waitErr = <-serveErr:
			case waitErr = <-waitForCompletionCh(ctrl, waitCtx):
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)

			if waitErr != nil {
				return fmt.Errorf("controller: %w", waitErr)
			}

			result := ctrl.Aggregate()
			fmt.Fprintln(cmd.OutOrStdout())
			report.WriteConsole(cmd.OutOrStdout(), result)

			if out != "" {
				if err := report.WriteJSON(out, result); err != nil {
					return err
				}
			}
			assertionResults, aerr := checkAssertions(cmd, s.Asserts, result)
			if htmlOut != "" {
				if err := report.WriteHTML(htmlOut, result, assertionResults); err != nil {
					return err
				}
			}
			return aerr
		},
	}
	c.Flags().StringVar(&walletsPath, "wallets", "wallets.json", "keystore file to shard across workers")
	c.Flags().StringVar(&listen, "listen", ":7700", "address to listen on for worker connections")
	c.Flags().IntVar(&numWorkers, "workers", 1, "number of worker processes to wait for")
	c.Flags().StringVar(&out, "out", "", "write aggregated JSON results to this path")
	c.Flags().StringVar(&htmlOut, "html", "", "write a self-contained aggregated HTML report to this path")
	c.Flags().DurationVar(&timeout, "timeout", 0, "give up waiting for workers to finish after this long (0 = wait indefinitely)")
	c.Flags().StringVar(&password, "password", "", "password for an encrypted keystore (or set "+keystorePasswordEnvVar+")")
	return c
}

func waitForCompletionCh(ctrl *distributed.Controller, ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- ctrl.WaitForCompletion(ctx) }()
	return ch
}
