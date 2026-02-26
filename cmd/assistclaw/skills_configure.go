package main

// skills_configure.go — "assistclaw skills configure" interactive TUI
// Also reused by onboarding so both flows are identical.
//
// Usage:
//   assistclaw skills configure
//   assistclaw skills configure --tag productivity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/skills"
)

// ─────────────────────────────────────────────
// Cobra subcommand
// ─────────────────────────────────────────────

func skillsConfigureCmd(gf *globalFlags) *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Interactive TUI to select, install, and manage skills",
		Long: `Open an interactive multi-select TUI to choose which skills your agent should use.
You can also add custom skills from a local path or URL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			return RunSkillsConfigure(cfg, gf.configPath, tag)
		},
	}
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Pre-filter skills by tag (e.g. productivity, ai, macos)")
	return cmd
}

// ─────────────────────────────────────────────
// RunSkillsConfigure — called from both CLI and onboarding
// ─────────────────────────────────────────────

// RunSkillsConfigure runs the interactive skill selection TUI.
// It returns when the user confirms their selection.
// cfgPath is the path to the config file to update (can be "" to skip config update).
func RunSkillsConfigure(cfg *config.Config, cfgPath string, tagFilter string) error {
	ctx := context.Background()

	home, _ := os.UserHomeDir()
	bundledDir := filepath.Join(home, ".assistclaw", "skills", "bundled")
	customDir := filepath.Join(home, ".assistclaw", "skills", "custom")
	_ = os.MkdirAll(bundledDir, 0o755)
	_ = os.MkdirAll(customDir, 0o755)

	mp := skills.NewMarketplace(bundledDir, customDir)

	// Fetch marketplace index
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render("  Fetching skill catalog..."))
	index, err := mp.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("could not load skill catalog: %w", err)
	}

	// Build currently installed set
	installedReg := skills.NewRegistry()
	_ = installedReg.LoadAll(ctx, customDir)
	enabledSet := make(map[string]bool)
	for _, s := range cfg.Agent.EnabledSkills {
		enabledSet[s] = true
	}

	// Filter by tag if requested
	entries := index.Skills
	if tagFilter != "" {
		var filtered []skills.MarketplaceEntry
		for _, e := range entries {
			for _, t := range e.Tags {
				if strings.EqualFold(t, tagFilter) {
					filtered = append(filtered, e)
					break
				}
			}
		}
		entries = filtered
		if len(entries) == 0 {
			return fmt.Errorf("no skills found for tag %q", tagFilter)
		}
	}

	// Build options list — pre-check enabled/installed skills
	type skillOption struct {
		name    string
		enabled bool
	}

	const customSkillSentinel = "__custom__"
	const skipSentinel = "__skip__"

	var options []huh.Option[string]
	var defaultSelected []string

	for _, e := range entries {
		label := e.Name
		if e.Emoji != "" {
			label = e.Emoji + "  " + e.Name
		}
		// Show install status
		_, isInstalled := installedReg.Get(e.Name)
		switch {
		case isInstalled && enabledSet[e.Name]:
			label += "  ✔"
		case isInstalled:
			label += "  (installed, disabled)"
		}
		label += "\n     " + truncateStr(e.Description, 72)

		options = append(options, huh.NewOption(label, e.Name))

		// Pre-select if currently enabled
		if enabledSet[e.Name] || (len(cfg.Agent.EnabledSkills) == 0 && isInstalled) {
			defaultSelected = append(defaultSelected, e.Name)
		}
	}

	// Add custom skill option at the bottom
	options = append(options, huh.NewOption(
		"＋  Add custom skill  (local path or URL)",
		customSkillSentinel,
	))

	var selectedNames []string
	var customPath string
	var addCustom bool

	theme := huh.ThemeCatppuccin()

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Render("  🛠  Configure Agent Skills")

	fmt.Println("\n" + header)
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
		Render("  Space to toggle · Enter to confirm · ↑↓ to navigate\n"))

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills to enable").
				Description("Bundled skills will be downloaded automatically if not installed.").
				Options(options...).
				Value(&selectedNames),
		),
	).WithTheme(theme)

	if err := form.Run(); err != nil {
		return fmt.Errorf("cancelled")
	}

	// Check if user picked the custom sentinel
	var finalNames []string
	for _, n := range selectedNames {
		if n == customSkillSentinel {
			addCustom = true
		} else if n != skipSentinel {
			finalNames = append(finalNames, n)
		}
	}

	// Custom skill form
	if addCustom {
		customForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Custom skill path or URL").
					Description("Local dir: /path/to/my-skill   GitHub URL: https://github.com/user/skill").
					Placeholder("/path/to/skill  or  https://github.com/user/skill").
					Value(&customPath),
			),
		).WithTheme(theme)

		if err := customForm.Run(); err == nil && customPath != "" {
			fmt.Printf("  Installing custom skill from %s...\n", customPath)
			dest, err := installCustomSkill(ctx, mp, customPath)
			if err != nil {
				fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
					Render(fmt.Sprintf("  ✗ Custom skill failed: %v", err)))
			} else {
				name := filepath.Base(dest)
				finalNames = append(finalNames, name)
				fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).
					Render(fmt.Sprintf("  ✔ Custom skill %q added", name)))
			}
		}
	}

	if len(finalNames) == 0 {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).
			Render("  No skills selected — skipping."))
		return nil
	}

	// Install selected skills that aren't already installed
	fmt.Println()
	var installedOK []string
	for _, name := range finalNames {
		if _, ok := installedReg.Get(name); ok {
			installedOK = append(installedOK, name)
			continue
		}
		fmt.Printf("  Installing %s...", name)
		dest, err := mp.Install(ctx, name)
		if err != nil {
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
				Render(fmt.Sprintf(" ✗ failed: %v", err)))
		} else {
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).
				Render(" ✔"))
			_ = dest
			installedOK = append(installedOK, name)
		}
	}

	// Update config
	if cfgPath != "" && len(installedOK) > 0 {
		for _, name := range installedOK {
			if err := toggleSkillInConfig(cfgPath, name, true); err != nil {
				fmt.Printf("  ⚠ Could not update config for %s: %v\n", name, err)
			}
		}
	}

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42")).
		Render(fmt.Sprintf("  ✔ %d skill(s) active", len(installedOK))))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
		Render("  Restart the agent to apply changes: assistclaw restart"))
	fmt.Println()

	return nil
}

// installCustomSkill handles both local path and URL installs.
func installCustomSkill(ctx context.Context, mp *skills.Marketplace, pathOrURL string) (string, error) {
	// Local path
	if !strings.HasPrefix(pathOrURL, "http") {
		expanded := expandHome(pathOrURL)
		return mp.InstallFromPath(expanded)
	}
	// Remote URL — use mp.Install which handles GitHub URLs
	return mp.Install(ctx, pathOrURL)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
