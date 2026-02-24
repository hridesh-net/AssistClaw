package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		provider     string
		apiKey       string
		baseURL      string
		apiVersion   string
		awsRegion    string
		awsProfile   string
		awsAccessKey string
		awsSecretKey string

		// Gateway
		gwMode string // loopback, lan, tailscale
		tsMode string // off, serve, funnel
		gwPort int    = 18790
		gwHost string = "127.0.0.1"

		// Channels
		selectedChannels []string
		tgBotToken       string
		dcBotToken       string
		slackBotToken    string
		slackAppToken    string
		waSessionID      string

		// Skills
		selectedSkills []string

		tpl string
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
		var bedrockAuthMode string
		awsRegion = "us-east-1"

		formBedrockAuth := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("How will you authenticate with AWS Bedrock?").
					Options(
						huh.NewOption("Native Bedrock API Key", "api_key"),
						huh.NewOption("AWS IAM Security Keys", "iam_keys"),
						huh.NewOption("AWS Profile (~/.aws/credentials)", "profile"),
					).
					Value(&bedrockAuthMode),
			),
		).WithTheme(theme)

		if err := formBedrockAuth.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}

		form2Fields = append(form2Fields, huh.NewInput().
			Title("Enter AWS Region").
			Value(&awsRegion))

		switch bedrockAuthMode {
		case "api_key":
			form2Fields = append(form2Fields, huh.NewInput().
				Title("Enter AWS Bedrock API Key").
				Password(true).
				Value(&apiKey))
		case "iam_keys":
			form2Fields = append(form2Fields,
				huh.NewInput().
					Title("Enter AWS Access Key ID").
					Value(&awsAccessKey),
				huh.NewInput().
					Title("Enter AWS Secret Access Key").
					Password(true).
					Value(&awsSecretKey),
			)
		case "profile":
			awsProfile = "default"
			form2Fields = append(form2Fields, huh.NewInput().
				Title("Enter AWS Profile").
				Value(&awsProfile))
		}
	}

	if len(form2Fields) > 0 {
		form2 := huh.NewForm(huh.NewGroup(form2Fields...)).WithTheme(theme)
		if err := form2.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	}

	// Phase 3: Gateway & Remote Access
	formGateway := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Gateway & Remote Access").
				Description("How would you like to expose AssistClaw control plane?").
				Options(
					huh.NewOption("Local Only (127.0.0.1)", "loopback"),
					huh.NewOption("Local Network (LAN)", "lan"),
					huh.NewOption("Tailscale (Recommended for Remote)", "tailscale"),
				).
				Value(&gwMode),
		),
	).WithTheme(theme)

	if err := formGateway.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	if gwMode == "tailscale" {
		formTailscale := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Tailscale Mode").
					Options(
						huh.NewOption("Tailscale Serve (Secure Private Link)", "serve"),
						huh.NewOption("Tailscale Funnel (Public Internet)", "funnel"),
						huh.NewOption("Tailscale IP Only (Private Tailnet)", "off"),
					).
					Value(&tsMode),
			),
		).WithTheme(theme)
		if err := formTailscale.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	} else if gwMode == "lan" {
		gwHost = "0.0.0.0"
	}

	// Phase 4: Messaging Channels
	formChannels := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Messaging Channels").
				Description("Which platforms should AssistClaw connect to?").
				Options(
					huh.NewOption("Telegram", "telegram"),
					huh.NewOption("Discord", "discord"),
					huh.NewOption("Slack", "slack"),
					huh.NewOption("WhatsApp (Beta)", "whatsapp"),
				).
				Value(&selectedChannels),
		),
	).WithTheme(theme)

	if err := formChannels.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	for _, ch := range selectedChannels {
		var chFields []huh.Field
		switch ch {
		case "telegram":
			chFields = append(chFields, huh.NewInput().Title("Telegram Bot Token").Password(true).Value(&tgBotToken))
		case "discord":
			chFields = append(chFields, huh.NewInput().Title("Discord Bot Token").Password(true).Value(&dcBotToken))
		case "slack":
			chFields = append(chFields,
				huh.NewInput().Title("Slack Bot Token (xoxb-...)").Password(true).Value(&slackBotToken),
				huh.NewInput().Title("Slack App Token (xapp-...)").Password(true).Value(&slackAppToken),
			)
		case "whatsapp":
			chFields = append(chFields, huh.NewInput().Title("WhatsApp Session ID (Optional)").Description("Leave blank for new session scanning").Value(&waSessionID))
		}

		if len(chFields) > 0 {
			formCh := huh.NewForm(huh.NewGroup(chFields...)).WithTheme(theme)
			if err := formCh.Run(); err != nil {
				return fmt.Errorf("onboarding interrupted")
			}
		}
	}

	// Phase 5: Skills Discovery
	// For now we just list standard ones or common ones
	formSkills := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Enable Skills").
				Description("Enable specialized capabilities for your agent.").
				Options(
					huh.NewOption("Browser Control", "browser"),
					huh.NewOption("System Administration", "sysadmin"),
					huh.NewOption("Project Management", "pm"),
					huh.NewOption("Development Assistant", "dev"),
				).
				Value(&selectedSkills),
		),
	).WithTheme(theme)

	if err := formSkills.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	// Apply defaults if inputs were left blank
	if baseURL == "" && needsBaseURL[provider] != "" {
		baseURL = needsBaseURL[provider]
	}
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}

	// Build the final YAML template
	var sb strings.Builder
	sb.WriteString("# AssistClaw Configuration\nversion: 1\n\n")

	// Gateway config
	sb.WriteString("gateway:\n")
	sb.WriteString(fmt.Sprintf("  host: \"%s\"\n", gwHost))
	sb.WriteString(fmt.Sprintf("  port: %d\n", gwPort))
	if gwMode == "tailscale" {
		sb.WriteString("  bind: \"tailnet\"\n")
		sb.WriteString("  tailscale:\n")
		sb.WriteString(fmt.Sprintf("    mode: \"%s\"\n", tsMode))
	} else {
		sb.WriteString(fmt.Sprintf("  bind: \"%s\"\n", gwMode))
	}
	sb.WriteString("\n")

	// Provider config
	sb.WriteString("agent:\n")
	sb.WriteString("  max_iterations: 64\n")
	if len(selectedSkills) > 0 {
		sb.WriteString("  enabled_skills:\n")
		for _, s := range selectedSkills {
			sb.WriteString(fmt.Sprintf("    - \"%s\"\n", s))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("providers:\n")
	switch provider {
	case "openai":
		sb.WriteString(fmt.Sprintf("  openai:\n    api_key: \"%s\"\n    default_model: \"gpt-4o-mini\"\n", apiKey))
	case "anthropic":
		sb.WriteString(fmt.Sprintf("  anthropic:\n    api_key: \"%s\"\n    default_model: \"claude-3-5-haiku-20241022\"\n", apiKey))
	case "ollama":
		sb.WriteString(fmt.Sprintf("  ollama:\n    base_url: \"%s\"\n    default_model: \"llama3.2\"\n", baseURL))
	case "bedrock":
		sb.WriteString("  bedrock:\n")
		sb.WriteString(fmt.Sprintf("    region: \"%s\"\n", awsRegion))
		if awsProfile != "" {
			sb.WriteString(fmt.Sprintf("    profile: \"%s\"\n", awsProfile))
		}
		if apiKey != "" {
			sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", apiKey))
		}
		if awsAccessKey != "" {
			sb.WriteString(fmt.Sprintf("    access_key_id: \"%s\"\n", awsAccessKey))
			sb.WriteString(fmt.Sprintf("    secret_access_key: \"%s\"\n", awsSecretKey))
		}
		sb.WriteString("    default_model: \"anthropic.claude-3-5-haiku-20241022-v1:0\"\n")
	case "azure":
		sb.WriteString("  azure_openai:\n")
		sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", apiKey))
		sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", baseURL))
		sb.WriteString(fmt.Sprintf("    api_version: \"%s\"\n", apiVersion))
		sb.WriteString("    default_model: \"gpt-4o\"\n")
	default:
		sb.WriteString(fmt.Sprintf("  %s:\n", provider))
		if apiKey != "" {
			sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", apiKey))
		}
		if baseURL != "" {
			sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", baseURL))
		}
		sb.WriteString("    default_model: \"default\"\n")
	}
	sb.WriteString("\n")

	// Channels config
	if len(selectedChannels) > 0 {
		sb.WriteString("channels:\n")
		for _, ch := range selectedChannels {
			switch ch {
			case "telegram":
				sb.WriteString(fmt.Sprintf("  telegram:\n    bot_token: \"%s\"\n", tgBotToken))
			case "discord":
				sb.WriteString(fmt.Sprintf("  discord:\n    bot_token: \"%s\"\n", dcBotToken))
			case "slack":
				sb.WriteString(fmt.Sprintf("  slack:\n    bot_token: \"%s\"\n    app_token: \"%s\"\n", slackBotToken, slackAppToken))
			case "whatsapp":
				sb.WriteString("  whatsapp:\n")
				if waSessionID != "" {
					sb.WriteString(fmt.Sprintf("    session_id: \"%s\"\n", waSessionID))
				} else {
					sb.WriteString("    session_id: \"default\"\n")
				}
			}
		}
		sb.WriteString("\n")
	}

	// Routing config
	sb.WriteString("routing:\n")
	defaultRoute := fmt.Sprintf("%s/default", provider)
	switch provider {
	case "openai":
		defaultRoute = "openai/gpt-4o-mini"
	case "anthropic":
		defaultRoute = "anthropic/claude-3-5-haiku-20241022"
	case "bedrock":
		defaultRoute = "bedrock/anthropic.claude-3-5-haiku-20241022-v1:0"
	}
	sb.WriteString(fmt.Sprintf("  default: \"%s\"\n", defaultRoute))

	tpl = sb.String()

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
