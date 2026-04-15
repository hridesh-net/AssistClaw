package main

import (
	"context"
	"fmt"
	"io"
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
Default URL: AssistClaw release asset (localintel.DefaultGGUFURL); override with --url or ASSISTCLAW_LOCAL_GEMMA_GGUF_URL.
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
			if !force {
				if src, ok := discoverBundledGGUF(path); ok {
					if err := copyFileAtomic(src, path); err != nil {
						return fmt.Errorf("local-intel setup: copy bundled gguf: %w", err)
					}
					fmt.Fprintln(os.Stderr, "local-intel setup: using local bundled GGUF from", src)
				}
			}
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
	cmd.Flags().StringVar(&url, "url", "", "GGUF URL (default: ASSISTCLAW_LOCAL_GEMMA_GGUF_URL or AssistClaw release asset)")
	cmd.Flags().StringVar(&ggufPath, "gguf-path", "", "Destination GGUF path (default: <state_dir>/models/gemma-4-e2b-it.gguf)")
	cmd.Flags().StringVar(&sha256, "sha256", "", "Optional SHA-256 hex digest to verify download")
	cmd.Flags().BoolVar(&force, "force", false, "Force re-download even if destination file exists")
	return cmd
}

// discoverBundledGGUF checks common local "models/" locations before network download.
// Priority: explicit canonical names, then first *.gguf match in cwd/models or executable-dir/models.
func discoverBundledGGUF(dst string) (string, bool) {
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 8)
	add := func(p string) {
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		candidates = append(candidates, p)
	}
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	preferred := []string{"gemma-4-e2b-it.gguf", "gemma-2-2b-it-Q4_K_M.gguf"}
	for _, root := range []string{
		filepath.Join(cwd, "models"),
		filepath.Join(exeDir, "models"),
	} {
		for _, n := range preferred {
			add(filepath.Join(root, n))
		}
		if ents, err := os.ReadDir(root); err == nil {
			for _, e := range ents {
				if e.IsDir() {
					continue
				}
				name := strings.ToLower(e.Name())
				if strings.HasSuffix(name, ".gguf") {
					add(filepath.Join(root, e.Name()))
				}
			}
		}
	}
	dst = filepath.Clean(dst)
	for _, p := range candidates {
		if filepath.Clean(p) == dst {
			continue
		}
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() && st.Size() > 0 {
			return p, true
		}
	}
	return "", false
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".copy"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
