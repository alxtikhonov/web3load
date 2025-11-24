package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/spf13/cobra"

	"github.com/web3load/web3load/internal/chain/evm"
	"github.com/web3load/web3load/internal/wallet"
)

func newWalletsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "wallets", Short: "Generate and fund test wallets"}
	cmd.AddCommand(newWalletsGenerateCmd())
	cmd.AddCommand(newWalletsFundCmd())
	return cmd
}

func newWalletsGenerateCmd() *cobra.Command {
	var count int
	var out string
	c := &cobra.Command{
		Use:   "generate",
		Short: "Generate N wallets into a keystore file (plaintext in v0.1 — see docs/security.md)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ks, err := wallet.Generate(count)
			if err != nil {
				return err
			}
			if err := ks.Save(out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "generated %d wallets -> %s\n", len(ks.Wallets), out)
			fmt.Fprintln(cmd.OutOrStdout(), "WARNING: v0.1 keystores are plaintext. Do not commit this file or use it on a network holding real funds.")
			return nil
		},
	}
	c.Flags().IntVar(&count, "count", 10, "number of wallets to generate")
	c.Flags().StringVar(&out, "out", "wallets.json", "output keystore path")
	return c
}

func newWalletsFundCmd() *cobra.Command {
	var walletsPath, rpcURL, fromKey, native string
	var chainID int64
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "fund",
		Short: "Fund every wallet in a keystore with native currency from a funder key",
		RunE: func(cmd *cobra.Command, args []string) error {
			ks, err := wallet.Load(walletsPath)
			if err != nil {
				return err
			}
			amount, ok := new(big.Int).SetString(native, 10)
			if !ok {
				return fmt.Errorf("--native must be an integer amount in wei, got %q", native)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			adapter, err := evm.Dial(ctx, rpcURL, chainID)
			if err != nil {
				return err
			}

			results := wallet.Fund(ctx, adapter, fromKey, ks.Wallets, amount, timeout)
			var failed int
			for _, r := range results {
				if r.Err != nil {
					failed++
					fmt.Fprintf(cmd.OutOrStdout(), "FAIL  %s: %v\n", r.Address.Hex(), r.Err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "funded %d/%d wallets\n", len(results)-failed, len(results))
			if failed > 0 {
				return fmt.Errorf("%d wallets failed to fund", failed)
			}
			return nil
		},
	}
	c.Flags().StringVar(&walletsPath, "wallets", "wallets.json", "keystore file to fund")
	c.Flags().StringVar(&rpcURL, "rpc-url", "http://127.0.0.1:8545", "EVM RPC endpoint")
	c.Flags().Int64Var(&chainID, "chain-id", 31337, "expected chain id (refuses to run on a mismatch)")
	c.Flags().StringVar(&fromKey, "from", "", "funder private key (hex, 0x-prefixed)")
	c.Flags().StringVar(&native, "native", "1000000000000000000", "amount to send to each wallet, in wei")
	c.Flags().DurationVar(&timeout, "confirm-timeout", 30*time.Second, "per-transfer confirmation timeout")
	_ = c.MarkFlagRequired("from")
	return c
}
