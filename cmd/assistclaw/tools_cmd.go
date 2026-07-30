package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/assistclaw/assistclaw/internal/autotool"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// tools command
// ─────────────────────────────────────────────

func toolsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Short: "List all tools available to the agent"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all tools: built-in, skill, and auto-generated",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			path := gf.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			cfg, err := loadConfig(path, log)
			if err != nil {
				return err
			}

			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			header := func(s string) string { return tui.Style(tui.ColorNeon, true, s) }

			// ── Section 1: Built-in tools ──────────────────────────────────────
			fmt.Println(header("\n⚡ Built-in System Tools") + dim("  (always available)"))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			builtins := []struct{ name, desc string }{
				{"bash", "Execute any shell command (mkdir, git, npm, pip, compile, run tests…)"},
				{"write_file", "Create or overwrite any file — source code, configs, scripts"},
				{"read_file", "Read file contents (with optional line range)"},
				{"list_dir", "Browse directory contents, optionally recursive"},
				{"grep", "Search patterns across files (regex, case-insensitive modes)"},
				{"web_fetch", "Fetch text content from a URL (docs, APIs, READMEs)"},
				{"browser_navigate", "Open a URL in a real browser session"},
				{"browser_screenshot", "Capture a screenshot of the browser"},
				{"memory_search", "Search episodic + semantic (vector) memory"},
				{"memory_get", "Read a specific line range from a memory file"},
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, t := range builtins {
				fmt.Fprintf(w, "  %s\t%s\n", prim(t.name), dim(t.desc))
			}
			w.Flush()

			// ── Section 2: Skill tools ─────────────────────────────────────────
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			skillReg := skills.NewRegistry()
			_ = skillReg.LoadAll(context.Background(), customDir)
			allSkills := skillReg.List()

			var skillToolCount int
			for _, s := range allSkills {
				skillToolCount += len(s.Tools)
			}

			fmt.Println(header("\n🧠 Skill Tools") + dim(fmt.Sprintf("  (%d installed skills)", len(allSkills))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			if skillToolCount == 0 {
				fmt.Println(dim("  No skill tools installed yet."))
				fmt.Println(dim("  Run: assistclaw skills install <name>"))
			} else {
				w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, s := range allSkills {
					for _, t := range s.Tools {
						fmt.Fprintf(w2, "  %s\t%s\t%s\n",
							prim(t.Name),
							dim("["+s.Name+"]"),
							dim(t.Description))
					}
				}
				w2.Flush()
			}

			// ── Section 3: Auto-generated tools ───────────────────────────────
			creator, err := autotool.NewCreator(autotool.CreatorConfig{
				ToolsDir: cfg.Agent.ToolsDir,
				VenvPath: filepath.Join(cfg.StateDir, "venv"),
				Timeout:  30,
			}, log)
			var autoList []autotool.ToolMeta
			if err == nil {
				autoList, _ = creator.List()
			}

			fmt.Println(header("\n🔧 Auto-generated Tools") + dim(fmt.Sprintf("  (%d generated)", len(autoList))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			if len(autoList) == 0 {
				fmt.Println(dim("  No auto-generated tools yet."))
				fmt.Println(dim("  Ask the agent to create one — it uses 'bash' and 'write_file' automatically."))
			} else {
				w3 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, t := range autoList {
					fmt.Fprintf(w3, "  %s\t%s\t%s\n",
						prim(t.Name),
						dim(t.CreatedAt.Format("2006-01-02")),
						dim(t.Description))
				}
				w3.Flush()
			}

			fmt.Println()
			return nil
		},
	})
	return cmd
}
