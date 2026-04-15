//go:build assistclaw_localgemma

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/internal/localintel"
)

func localgemmaCmd(_ *globalFlags) *cobra.Command {
	root := &cobra.Command{
		Use:   "localgemma",
		Short: "Run Gemma 4 E2B locally (in-process, optional embedded GGUF)",
	}

	run := &cobra.Command{
		Use:   "run",
		Short: "Single-turn completion against embedded or configured GGUF",
		RunE: func(cmd *cobra.Command, args []string) error {
			user, _ := cmd.Flags().GetString("user")
			if user == "" {
				return fmt.Errorf("flag --user is required")
			}
			system, _ := cmd.Flags().GetString("system")
			maxTok, _ := cmd.Flags().GetInt("max-tokens")
			gguf, _ := cmd.Flags().GetString("gguf")

			opt := localintel.Options{GGUFPath: gguf}
			eng, err := localintel.Open(opt)
			if err != nil {
				return err
			}
			defer eng.Close()

			out, err := eng.Complete(context.Background(), localintel.Request{
				System:    system,
				User:      user,
				MaxTokens: maxTok,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, out)
			return nil
		},
	}
	run.Flags().String("system", "", "Optional system preamble")
	run.Flags().String("user", "", "User message")
	run.Flags().Int("max-tokens", 256, "Maximum new tokens to generate")
	run.Flags().String("gguf", "", "Override GGUF path (otherwise uses ASSISTCLAW_LOCAL_GEMMA_GGUF or embedded weights)")

	info := &cobra.Command{
		Use:   "info",
		Short: "Show whether embedded weights are present in this binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "embedded bytes: %d\n", len(localintel.Gemma4E2BGGUF))
			if p := os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF"); p != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "ASSISTCLAW_LOCAL_GEMMA_GGUF: %s\n", p)
			}
			return nil
		},
	}

	root.AddCommand(run, info)
	return root
}
