package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func onboardCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Run the interactive first-time setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := gf.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			return runOnboarding(path)
		},
	}
}

func runOnboarding(configPath string) error {
	var (
		provider string
		apiKey   string
		tpl      string
	)

	// Custom theme colors to match OpenClaw
	theme := huh.ThemeBase()
	theme.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	theme.Focused.SelectedOption = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	theme.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("  Welcome to AssistClaw 🐾"))
	fmt.Println(lipgloss.NewStyle().
		Faint(true).
		Render("  Let's configure your autonomous agent environment."))
	fmt.Println()

	form1 := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which primary AI provider would you like to use?").
				Options(
					huh.NewOption("Anthropic (Recommended)", "anthropic"),
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Ollama (Local / Free)", "ollama"),
				).
				Value(&provider),
		),
	).WithTheme(theme)

	if err := form1.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	if provider != "ollama" {
		form2 := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your API Key").
					Description("This is stored safely in your local ~/.assistclaw configuration.").
					Password(true).
					Value(&apiKey),
			),
		).WithTheme(theme)

		if err := form2.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	}

	switch provider {
	case "openai":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  openai:
    api_key: "%s"
    default_model: "gpt-4o-mini"

routing:
  default: "openai/gpt-4o-mini"
`, apiKey)

	case "ollama":
		tpl = `# AssistClaw Configuration
version: 1

providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    default_model: "llama3.2"

routing:
  default: "ollama/llama3.2"
`

	case "anthropic":
		fallthrough
	default:
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  anthropic:
    api_key: "%s"
    default_model: "claude-3-5-haiku-20241022"

routing:
  default: "anthropic/claude-3-5-haiku-20241022"
`, apiKey)
	}

	// Dump empty file and create dirs so it works even if dir doesn't exist
	if err := config.InitializeWorkspace(configPath); err != nil {
		return err
	}

	// Ensure parent dir exists
	_ = os.MkdirAll(filepath.Dir(configPath), 0o755)

	if err := os.WriteFile(configPath, []byte(tpl), 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	boldStyle := lipgloss.NewStyle().Bold(true)

	fmt.Println()
	fmt.Println(successStyle.Render("✔ You're all set!"))
	fmt.Printf("Your configuration was saved to: %s\n\n", configPath)
	fmt.Println("Try running your first command:")
	fmt.Println(boldStyle.Render("  assistclaw agent -m \"Say hello!\""))
	fmt.Println()

	return nil
}
