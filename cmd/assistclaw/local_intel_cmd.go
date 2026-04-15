package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/internal/localintel"
)

func localIntelCmd(gf *globalFlags) *cobra.Command {
	var stateDir string
	cmd := &cobra.Command{
		Use:     "local-intel",
		Aliases: []string{"localintel"},
		Short:   "Bootstrap or inspect optional on-device Gemma GGUF setup",
	}
	cmd.PersistentFlags().StringVar(&stateDir, "state-dir", "",
		"AssistClaw state directory (e.g. ~/.assistclaw); uses env-style defaults without assistclaw.yaml")
	cmd.AddCommand(localIntelSetupCmd(gf, &stateDir))
	return cmd
}

func localIntelSetupCmd(gf *globalFlags, stateDir *string) *cobra.Command {
	var (
		url      string
		ggufPath string
		sha256   string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Download GGUF and print ready-to-merge local_intel config snippet",
		Long: `Downloads a Gemma-compatible GGUF into a managed path and prints a config snippet.

Default output path: <state_dir>/models/gemma-4-e2b-it.gguf
Default URL: internal localintel.DefaultGGUFURL (override with --url or ASSISTCLAW_LOCAL_GEMMA_GGUF_URL).
Optional integrity check: --sha256 or ASSISTCLAW_LOCAL_GEMMA_GGUF_SHA256.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := mempalaceLoadCfg(gf, *stateDir)
			if err != nil {
				return err
			}
			path := strings.TrimSpace(ggufPath)
			if path == "" {
				path = strings.TrimSpace(cfg.Agent.LocalIntel.GGUFPath)
			}
			if path == "" {
				if env := strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF")); env != "" {
					path = env
				}
			}
			if path == "" {
				path = localintel.DefaultGGUFPath(cfg.StateDir)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("local-intel setup: mkdir destination: %w", err)
			}
			fmt.Fprintln(os.Stderr, "local-intel setup: state_dir", cfg.StateDir)
			fmt.Fprintln(os.Stderr, "local-intel setup: gguf destination", path)
			if !localintel.CompiledWithLocalGemma() {
				fmt.Fprintln(os.Stderr, "local-intel setup: warning: this binary was not built with assistclaw_localgemma; GGUF will be prepared but in-process Gemma remains unavailable until you install a localgemma-enabled build")
			}
			res, err := localintel.BootstrapGGUF(context.Background(), localintel.BootstrapOptions{
				StateDir: cfg.StateDir,
				GGUFPath: path,
				URL:      strings.TrimSpace(url),
				SHA256:   strings.TrimSpace(sha256),
				Force:    force,
				Progress: os.Stderr,
			})
			if err != nil {
				return err
			}
			if res.Downloaded {
				fmt.Fprintf(os.Stderr, "\nlocal-intel setup: downloaded %.1f MB\n", float64(res.Bytes)/(1024*1024))
			} else {
				fmt.Fprintln(os.Stderr, "local-intel setup: existing GGUF reused")
			}
			fmt.Println("Merge into assistclaw.yaml (or your generated config):")
			fmt.Println("agent:")
			fmt.Println("  local_intel:")
			fmt.Println("    enabled: true")
			fmt.Printf("    gguf_path: %q\n", res.Path)
			fmt.Println("    max_tokens: 256")
			fmt.Printf("    cache_dir: %q\n", filepath.Join(cfg.StateDir, "localintel"))
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "GGUF URL (default: ASSISTCLAW_LOCAL_GEMMA_GGUF_URL or built-in default)")
	cmd.Flags().StringVar(&ggufPath, "gguf-path", "", "Destination GGUF path (default: <state_dir>/models/gemma-4-e2b-it.gguf)")
	cmd.Flags().StringVar(&sha256, "sha256", "", "Optional SHA-256 hex digest to verify download")
	cmd.Flags().BoolVar(&force, "force", false, "Force re-download even if destination file exists")
	return cmd
}
