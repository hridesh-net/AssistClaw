package main

// skills_cmd.go — "assistclaw skills" subcommand suite
// Provides list, add, remove, enable, disable, install, info, marketplace.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func skillsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage, install, and browse agent skills",
		Long: `Manage installed skills, browse the marketplace, and install community skills.

Skills live in two directories:
  bundled:  skills shipped with AssistClaw  (~/.assistclaw/skills/bundled/)
  custom:   your active/custom skills        (~/.assistclaw/skills/custom/)

Run 'assistclaw skills configure' (or just 'assistclaw skills') for the interactive TUI.`,
		// Default: open the configure TUI when no subcommand is given
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			return RunSkillsConfigure(cfg, gf.configPath, "")
		},
	}

	cmd.AddCommand(
		skillsConfigureCmd(gf),
		skillsListCmd(gf),
		skillsAddCmd(gf),
		skillsRemoveCmd(gf),
		skillsEnableCmd(gf),
		skillsDisableCmd(gf),
		skillsInstallCmd(gf),
		skillsInfoCmd(gf),
		skillsMarketplaceCmd(gf),
	)
	return cmd
}

// ─────────────────────────────────────────────
// skills list
// ─────────────────────────────────────────────

func skillsListCmd(gf *globalFlags) *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")

			reg := skills.NewRegistry()
			_ = reg.LoadAll(context.Background(), customDir)

			all := reg.List()
			if len(all) == 0 && !showAll {
				fmt.Println("No skills installed yet.")
				fmt.Println("  Run: assistclaw skills marketplace  — to browse available skills")
				fmt.Println("  Run: assistclaw skills add <name>   — to install a bundled skill")
				return nil
			}

			enabledSet := make(map[string]bool)
			for _, s := range cfg.Agent.EnabledSkills {
				enabledSet[s] = true
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tDESCRIPTION")
			fmt.Fprintln(w, "────\t──────\t───────────")
			for _, s := range all {
				status := "active"
				if len(cfg.Agent.EnabledSkills) > 0 && !enabledSet[s.Name] {
					status = "inactive"
				}
				met, _ := reg.CheckRequirements(&s)
				if !met {
					status = "⚠ missing deps"
				}
				desc := s.Description
				if len(desc) > 70 {
					desc = desc[:67] + "…"
				}
				emoji := s.Metadata.OpenClaw.Emoji
				name := s.Name
				if emoji != "" {
					name = emoji + " " + name
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", name, status, desc)
			}
			w.Flush()

			if showAll {
				fmt.Printf("\nBundled (available to add): %s\n", bundledDir)
				bundleReg := skills.NewRegistry()
				_ = bundleReg.LoadAll(context.Background(), bundledDir)
				for _, s := range bundleReg.List() {
					if _, ok := reg.Get(s.Name); !ok {
						fmt.Printf("  + %s — %s\n", s.Name, s.Description)
					}
				}
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Also show bundled skills available to install")
	return cmd
}

// ─────────────────────────────────────────────
// skills add
// ─────────────────────────────────────────────

func skillsAddCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Copy a bundled skill into your active skills directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			mp := skills.NewMarketplace(bundledDir, customDir)

			fmt.Printf("Installing skill %q...\n", name)
			dest, err := mp.Install(context.Background(), name)
			if err != nil {
				return err
			}
			fmt.Printf("✔ Installed to %s\n", dest)

			// Auto-enable in config
			return toggleSkillInConfig(gf.configPath, name, true)
		},
	}
}

// ─────────────────────────────────────────────
// skills remove
// ─────────────────────────────────────────────

func skillsRemoveCmd(gf *globalFlags) *cobra.Command {
	var keepFiles bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Deactivate and optionally delete a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			if err := toggleSkillInConfig(gf.configPath, name, false); err != nil {
				return err
			}

			if !keepFiles {
				customDir := filepath.Join(cfg.StateDir, "skills", "custom", name)
				if _, err := os.Stat(customDir); err == nil {
					if err := os.RemoveAll(customDir); err != nil {
						return fmt.Errorf("failed to remove skill files: %w", err)
					}
					fmt.Printf("✔ Removed skill %q\n", name)
				}
			} else {
				fmt.Printf("✔ Skill %q disabled (files kept)\n", name)
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&keepFiles, "keep-files", false, "Disable skill without deleting its files")
	return cmd
}

// ─────────────────────────────────────────────
// skills enable / disable
// ─────────────────────────────────────────────

func skillsEnableCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Add a skill to enabled_skills in config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toggleSkillInConfig(gf.configPath, args[0], true); err != nil {
				return err
			}
			fmt.Printf("✔ Skill %q enabled\n", args[0])
			return nil
		},
	}
}

func skillsDisableCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Remove a skill from enabled_skills (keep files)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toggleSkillInConfig(gf.configPath, args[0], false); err != nil {
				return err
			}
			fmt.Printf("✔ Skill %q disabled\n", args[0])
			return nil
		},
	}
}

// ─────────────────────────────────────────────
// skills install
// ─────────────────────────────────────────────

func skillsInstallCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "install <url-or-name>",
		Short: "Install a skill from the marketplace, GitHub URL, or shorthand",
		Args:  cobra.ExactArgs(1),
		Example: `  assistclaw skills install notion
  assistclaw skills install github/my-skill-repo
  assistclaw skills install https://github.com/user/skill`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nameOrURL := args[0]
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			mp := skills.NewMarketplace(bundledDir, customDir)

			fmt.Printf("Installing %q...\n", nameOrURL)
			dest, err := mp.Install(context.Background(), nameOrURL)
			if err != nil {
				return err
			}
			name := filepath.Base(dest)
			fmt.Printf("✔ Installed to %s\n", dest)

			// Check and offer dependency install
			reg := skills.NewRegistry()
			_ = reg.LoadAll(context.Background(), dest)
			s, ok := reg.Get(name)
			if ok {
				met, missing := reg.CheckRequirements(s)
				if !met {
					fmt.Printf("⚠ Missing dependencies: %s\n", strings.Join(missing, ", "))
					fmt.Println("  Run: assistclaw onboard   — to install them interactively")
				}
			}

			return toggleSkillInConfig(gf.configPath, name, true)
		},
	}
}

// ─────────────────────────────────────────────
// skills info
// ─────────────────────────────────────────────

func skillsInfoCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show full metadata for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			reg := skills.NewRegistry()
			for _, dir := range []string{
				filepath.Join(cfg.StateDir, "skills", "custom"),
				filepath.Join(cfg.StateDir, "skills", "bundled"),
			} {
				_ = reg.LoadAll(context.Background(), dir)
			}

			s, ok := reg.Get(name)
			if !ok {
				return fmt.Errorf("skill %q not found", name)
			}

			emoji := s.Metadata.OpenClaw.Emoji
			if emoji != "" {
				fmt.Printf("%s %s\n", emoji, s.Name)
			} else {
				fmt.Println(s.Name)
			}
			fmt.Printf("Description: %s\n", s.Description)
			if s.Version != "" {
				fmt.Printf("Version:     %s\n", s.Version)
			}
			if s.Author != "" {
				fmt.Printf("Author:      %s\n", s.Author)
			}

			met, missing := reg.CheckRequirements(s)
			if met {
				fmt.Println("Status:      ✔ All dependencies met")
			} else {
				fmt.Printf("Status:      ⚠ Missing: %s\n", strings.Join(missing, ", "))
			}

			fmt.Printf("\nNodes (%d):\n", len(s.Nodes))
			for _, node := range s.Nodes {
				fmt.Printf("  • %-20s %s\n", node.Name, node.Summary)
			}

			if len(s.Tools) > 0 {
				fmt.Printf("\nTools (%d):\n", len(s.Tools))
				for _, t := range s.Tools {
					fmt.Printf("  • %-20s %s\n", t.Name, t.Description)
				}
			}

			return nil
		},
	}
}

// ─────────────────────────────────────────────
// skills marketplace
// ─────────────────────────────────────────────

func skillsMarketplaceCmd(gf *globalFlags) *cobra.Command {
	var query string
	var tag string
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Browse available skills from the marketplace",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			mp := skills.NewMarketplace(bundledDir, customDir)

			fmt.Println("Fetching marketplace index...")
			index, err := mp.FetchIndex(context.Background())
			if err != nil {
				return fmt.Errorf("marketplace unavailable: %w", err)
			}

			// Filter
			entries := index.Skills
			if query != "" {
				results, _ := mp.Search(context.Background(), query)
				entries = results
			}
			if tag != "" {
				var filtered []skills.MarketplaceEntry
				for _, e := range entries {
					for _, t := range e.Tags {
						if strings.EqualFold(t, tag) {
							filtered = append(filtered, e)
							break
						}
					}
				}
				entries = filtered
			}

			// Load installed skills to show install status
			reg := skills.NewRegistry()
			_ = reg.LoadAll(context.Background(), customDir)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "\n  AssistClaw Skill Marketplace  (%d skills)\n\n", len(entries))
			fmt.Fprintln(w, "  NAME\tTAGS\tSTATUS\tDESCRIPTION")
			fmt.Fprintln(w, "  ────\t────\t──────\t───────────")
			for _, e := range entries {
				status := ""
				if _, ok := reg.Get(e.Name); ok {
					status = "✔ installed"
				} else if e.Bundled {
					status = "available"
				} else {
					status = "community"
				}
				desc := e.Description
				if len(desc) > 65 {
					desc = desc[:62] + "…"
				}
				label := e.Name
				if e.Emoji != "" {
					label = e.Emoji + " " + e.Name
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", label, strings.Join(e.Tags, ","), status, desc)
			}
			w.Flush()
			fmt.Printf("\nInstall a skill: assistclaw skills install <name>\n")
			fmt.Printf("Filter by tag:  assistclaw skills marketplace --tag productivity\n")
			return nil
		},
	}
	cmd.Flags().StringVarP(&query, "search", "s", "", "Search query (name, description, or tag)")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Filter by tag (e.g. productivity, ai, macos)")
	return cmd
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// toggleSkillInConfig adds or removes a skill name from enabled_skills in the YAML config.
func toggleSkillInConfig(cfgPath string, skillName string, enable bool) error {
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	agentRaw, _ := root["agent"]
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		agentMap = make(map[string]any)
	}

	rawList, _ := agentMap["enabled_skills"].([]any)
	var enabled []string
	for _, v := range rawList {
		if s, ok := v.(string); ok {
			enabled = append(enabled, s)
		}
	}

	if enable {
		// Add if not present
		found := false
		for _, s := range enabled {
			if s == skillName {
				found = true
				break
			}
		}
		if !found {
			enabled = append(enabled, skillName)
		}
	} else {
		// Remove
		var filtered []string
		for _, s := range enabled {
			if s != skillName {
				filtered = append(filtered, s)
			}
		}
		enabled = filtered
	}

	agentMap["enabled_skills"] = enabled
	root["agent"] = agentMap

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}
