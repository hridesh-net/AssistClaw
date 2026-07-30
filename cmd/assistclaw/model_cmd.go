package main

import (
	"context"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// providers command
// ─────────────────────────────────────────────

func providersCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "providers", Short: "List and manage LLM providers"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all configured LLM providers and their models",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := provider.NewRegistry()
			registerProviders(ctx, cfg, reg, log) //nolint:errcheck
			for _, p := range reg.All() {
				models, err := p.ListModels(ctx)
				suffix := ""
				if err != nil {
					suffix = " (error: " + err.Error() + ")"
				}
				fmt.Printf("\n%s%s\n", p.Name(), suffix)
				for _, m := range models {
					local := ""
					if m.Local {
						local = " [local]"
					}
					fmt.Printf("  - %s%s\n", m.ID, local)
				}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Check connectivity to all configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := provider.NewRegistry()
			registerProviders(ctx, cfg, reg, log) //nolint:errcheck
			report := reg.CheckAll(ctx)
			for name, result := range report.Results {
				status := "✓"
				detail := ""
				if !result.OK {
					status = "✗"
					detail = " — " + result.Error
				}
				fmt.Printf("%s %s%s\n", status, name, detail)
			}
			return nil
		},
	})
	return cmd
}

// ─────────────────────────────────────────────
// embeddings command
// ─────────────────────────────────────────────

func embeddingsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "embed", Short: "Embed text using configured embedding models"}
	cmd.AddCommand(&cobra.Command{
		Use:   "text [text]",
		Short: "Embed a piece of text and show the vector dimensions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := embeddings.NewRegistry()
			registerEmbedders(ctx, cfg, reg, log)
			vec, err := reg.EmbedText(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Embedded %d-dimensional vector (showing first 8 dims): %v...\n", len(vec), vec[:min(8, len(vec))])
			return nil
		},
	})
	return cmd
}
