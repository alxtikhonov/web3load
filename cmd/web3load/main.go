// Command web3load is the CLI entrypoint: validate, run, wallets
// generate/fund, report, and the controller/worker distributed mode
// commands.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var logLevel, logFormat string

func main() {
	root := &cobra.Command{
		Use:   "web3load",
		Short: "Load testing for Web3 infrastructure — RPC, transactions, and smart contract flows.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return configureLogging(logLevel, logFormat)
		},
	}
	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	root.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format: text, json")

	root.AddCommand(newValidateCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newWalletsCmd())
	root.AddCommand(newReportCmd())
	root.AddCommand(newControllerCmd())
	root.AddCommand(newWorkerCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// configureLogging installs the process-wide slog logger. Diagnostic logs
// (nonce resyncs, per-step failures, stage transitions) go to stderr in
// this format/level; report output (console/JSON) always goes to stdout
// separately, so piping `web3load run ... > results.txt` never mixes the
// two.
func configureLogging(level, format string) error {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return fmt.Errorf("unknown --log-level %q (want debug|info|warn|error)", level)
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("unknown --log-format %q (want text|json)", format)
	}

	slog.SetDefault(slog.New(handler))
	return nil
}
