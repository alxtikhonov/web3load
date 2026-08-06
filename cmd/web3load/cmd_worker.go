package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/distributed"
)

func newWorkerCmd() *cobra.Command {
	var controllerURL, workerID string
	var progressInterval time.Duration
	c := &cobra.Command{
		Use:   "worker",
		Short: "Register with a controller and run this process's shard of its scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			if workerID == "" {
				host, _ := os.Hostname()
				workerID = fmt.Sprintf("%s-%d", host, os.Getpid())
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			fmt.Fprintf(cmd.OutOrStdout(), "worker %q connecting to %s (blocks until the whole cohort has joined)...\n", workerID, controllerURL)
			return distributed.Run(ctx, distributed.WorkerConfig{
				ControllerURL:    controllerURL,
				WorkerID:         workerID,
				ProgressInterval: progressInterval,
			})
		},
	}
	c.Flags().StringVar(&controllerURL, "controller", "http://127.0.0.1:7700", "controller base URL")
	c.Flags().StringVar(&workerID, "id", "", "worker id reported to the controller (default: hostname-pid)")
	c.Flags().DurationVar(&progressInterval, "progress-interval", 10*time.Second, "how often to push a progress snapshot to the controller; 0 disables it")
	return c
}
