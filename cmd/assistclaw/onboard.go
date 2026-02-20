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
		provider   string
		apiKey     string
		baseURL    string
		apiVersion string
		awsRegion  string
		awsProfile string
		tpl        string
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
					huh.NewOption("AWS Bedrock", "bedrock"),
					huh.NewOption("vLLM (Local / Custom)", "vllm"),
					huh.NewOption("LM Studio (Local)", "lmstudio"),
					huh.NewOption("Groq", "groq"),
					huh.NewOption("Mistral", "mistral"),
					huh.NewOption("OpenRouter", "openrouter"),
					huh.NewOption("Azure OpenAI", "azure"),
				).
				Value(&provider),
		),
	).WithTheme(theme)

	if err := form1.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	// Dynamic second step based on the chosen provider
	var form2Fields []huh.Field

	needsAPIKey := map[string]bool{
		"anthropic":  true,
		"openai":     true,
		"groq":       true,
		"mistral":    true,
		"openrouter": true,
		"azure":      true,
	}

	needsBaseURL := map[string]string{
		"ollama":   "http://localhost:11434",
		"vllm":     "http://localhost:8000/v1",
		"lmstudio": "http://localhost:1234/v1",
		"azure":    "https://YOUR_RESOURCE_NAME.openai.azure.com",
	}

	if needsAPIKey[provider] {
		form2Fields = append(form2Fields, huh.NewInput().
			Title("Enter your API Key").
			Description("This is stored safely in your local ~/.assistclaw configuration.").
			Password(true).
			Value(&apiKey))
	}

	if defaultURL, ok := needsBaseURL[provider]; ok {
		form2Fields = append(form2Fields, huh.NewInput().
			Title(fmt.Sprintf("Enter Base URL (Default: %s)", defaultURL)).
			Value(&baseURL))
	}

	if provider == "azure" {
		form2Fields = append(form2Fields, huh.NewInput().
			Title("Enter Azure API Version (e.g., 2024-02-15-preview)").
			Value(&apiVersion))
	}

	if provider == "bedrock" {
		awsRegion = "us-east-1"
		awsProfile = "default"
		form2Fields = append(form2Fields,
			huh.NewInput().
				Title("Enter AWS Region").
				Value(&awsRegion),
			huh.NewInput().
				Title("Enter AWS Profile").
				Value(&awsProfile),
		)
	}

	if len(form2Fields) > 0 {
		form2 := huh.NewForm(huh.NewGroup(form2Fields...)).WithTheme(theme)
		if err := form2.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	}

	// Apply defaults if inputs were left blank
	if baseURL == "" && needsBaseURL[provider] != "" {
		baseURL = needsBaseURL[provider]
	}
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}
	if awsProfile == "" {
		awsProfile = "default"
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
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  ollama:
    base_url: "%s"
    default_model: "llama3.2"

routing:
  default: "ollama/llama3.2"
`, baseURL)

	case "bedrock":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  bedrock:
    region: "%s"
    profile: "%s"
    default_model: "anthropic.claude-3-5-haiku-20241022-v1:0"

routing:
  default: "bedrock/anthropic.claude-3-5-haiku-20241022-v1:0"
`, awsRegion, awsProfile)

	case "vllm":
		fallthrough
	case "lmstudio":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  %s:
    base_url: "%s"
    default_model: "local-model"

routing:
  default: "%s/local-model"
`, provider, baseURL, provider)

	case "groq":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  groq:
    api_key: "%s"
    default_model: "llama-3.1-8b-instant"

routing:
  default: "groq/llama-3.1-8b-instant"
`, apiKey)

	case "mistral":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  mistral:
    api_key: "%s"
    default_model: "mistral-large-latest"

routing:
  default: "mistral/mistral-large-latest"
`, apiKey)

	case "openrouter":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  openrouter:
    api_key: "%s"
    default_model: "anthropic/claude-3-5-haiku-20241022:beta"

routing:
  default: "openrouter/anthropic/claude-3-5-haiku-20241022:beta"
`, apiKey)

	case "azure":
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  azure:
    api_key: "%s"
    base_url: "%s"
    api_version: "%s"
    default_model: "gpt-4o"

routing:
  default: "azure/gpt-4o"
`, apiKey, baseURL, apiVersion)

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
