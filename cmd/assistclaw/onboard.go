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
		provider          string
		apiKey            string
		baseURL           string
		apiVersion        string
		awsRegion         string
		awsProfile        string
		awsAccessKey      string
		awsSecretKey      string
		selectedModel     string
		secondaryProvider string

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

	// Model Selection
	modelChoices := map[string][]huh.Option[string]{
		"anthropic": {
			huh.NewOption("Claude 3.5 Sonnet (Best Balance)", "claude-3-5-sonnet-20241022"),
			huh.NewOption("Claude 3.5 Haiku (Fast & Cheap)", "claude-3-5-haiku-20241022"),
			huh.NewOption("Claude 3 Opus (Most Powerful)", "claude-3-opus-20240229"),
		},
		"openai": {
			huh.NewOption("GPT-4o (Most Capable)", "gpt-4o"),
			huh.NewOption("GPT-4o-mini (Fast & Cheap)", "gpt-4o-mini"),
			huh.NewOption("o1-preview (Reasoning)", "o1-preview"),
		},
		"ollama": {
			huh.NewOption("Llama 3.2 (3B)", "llama3.2"),
			huh.NewOption("Mistral (7B)", "mistral"),
			huh.NewOption("DeepSeek-R1 (Distill)", "deepseek-r1"),
		},
		"bedrock": {
			huh.NewOption("Claude 3.5 Sonnet v2", "anthropic.claude-3-5-sonnet-20241022-v2:0"),
			huh.NewOption("Claude 3.5 Haiku", "anthropic.claude-3-5-haiku-20241022-v1:0"),
			huh.NewOption("Llama 3.3 70B", "meta.llama3-3-70b-instruct-v1:0"),
		},
		"groq": {
			huh.NewOption("Llama 3.3 70B Versatile", "llama-3.3-70b-versatile"),
			huh.NewOption("Mixtral 8x7B", "mixtral-8x7b-32768"),
			huh.NewOption("DeepSeek-R1 70B", "deepseek-r1-distill-llama-70b"),
		},
	}

	if opts, ok := modelChoices[provider]; ok {
		formModel := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which specific model would you like to use by default?").
					Options(opts...).
					Value(&selectedModel),
			),
		).WithTheme(theme)
		if err := formModel.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	} else if provider != "" {
		formModel := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter Model ID (e.g., mistral-small-latest)").
					Value(&selectedModel),
			),
		).WithTheme(theme)
		if err := formModel.Run(); err != nil {
			return fmt.Errorf("onboarding interrupted")
		}
	}

	// Optional Secondary Provider
	formSecondary := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Would you like to configure a secondary / fallback provider?").
				Description("Useful for high availability or task-specific routing.").
				Options(
					huh.NewOption("None (Stick to primary)", "none"),
					huh.NewOption("Ollama (Local / Free)", "ollama"),
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Groq (Super Fast)", "groq"),
				).
				Value(&secondaryProvider),
		),
	).WithTheme(theme)
	if err := formSecondary.Run(); err != nil {
		return fmt.Errorf("onboarding interrupted")
	}

	var secondaryAPIKey string
	var secondaryBaseURL string
	if secondaryProvider != "none" && secondaryProvider != provider {
		var secFields []huh.Field
		if needsAPIKey[secondaryProvider] {
			secFields = append(secFields, huh.NewInput().
				Title(fmt.Sprintf("Enter API Key for %s", secondaryProvider)).
				Password(true).
				Value(&secondaryAPIKey))
		}
		if du, ok := needsBaseURL[secondaryProvider]; ok {
			secFields = append(secFields, huh.NewInput().
				Title(fmt.Sprintf("Enter Base URL for %s (Default: %s)", secondaryProvider, du)).
				Value(&secondaryBaseURL))
		}
		if len(secFields) > 0 {
			formSec := huh.NewForm(huh.NewGroup(secFields...)).WithTheme(theme)
			if err := formSec.Run(); err != nil {
				return fmt.Errorf("onboarding interrupted")
			}
		}
		if secondaryBaseURL == "" && needsBaseURL[secondaryProvider] != "" {
			secondaryBaseURL = needsBaseURL[secondaryProvider]
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
	writeProvider := func(p, ak, bu, model string) {
		switch p {
		case "openai":
			m := model
			if m == "" {
				m = "gpt-4o-mini"
			}
			sb.WriteString(fmt.Sprintf("  openai:\n    api_key: \"%s\"\n    default_model: \"%s\"\n", ak, m))
		case "anthropic":
			m := model
			if m == "" {
				m = "claude-3-5-haiku-20241022"
			}
			sb.WriteString(fmt.Sprintf("  anthropic:\n    api_key: \"%s\"\n    default_model: \"%s\"\n", ak, m))
		case "ollama":
			m := model
			if m == "" {
				m = "llama3.2"
			}
			sb.WriteString(fmt.Sprintf("  ollama:\n    base_url: \"%s\"\n    default_model: \"%s\"\n", bu, m))
		case "bedrock":
			sb.WriteString("  bedrock:\n")
			sb.WriteString(fmt.Sprintf("    region: \"%s\"\n", awsRegion))
			if awsProfile != "" {
				sb.WriteString(fmt.Sprintf("    profile: \"%s\"\n", awsProfile))
			}
			if ak != "" {
				sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", ak))
			}
			if awsAccessKey != "" {
				sb.WriteString(fmt.Sprintf("    access_key_id: \"%s\"\n", awsAccessKey))
				sb.WriteString(fmt.Sprintf("    secret_access_key: \"%s\"\n", awsSecretKey))
			}
			m := model
			if m == "" {
				m = "anthropic.claude-3-5-haiku-20241022-v1:0"
			}
			sb.WriteString(fmt.Sprintf("    default_model: \"%s\"\n", m))
		case "azure":
			sb.WriteString("  azure_openai:\n")
			sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", ak))
			sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", bu))
			sb.WriteString(fmt.Sprintf("    api_version: \"%s\"\n", apiVersion))
			m := model
			if m == "" {
				m = "gpt-4o"
			}
			sb.WriteString(fmt.Sprintf("    default_model: \"%s\"\n", m))
		case "groq":
			m := model
			if m == "" {
				m = "llama-3.3-70b-versatile"
			}
			sb.WriteString(fmt.Sprintf("  groq:\n    api_key: \"%s\"\n    default_model: \"%s\"\n", ak, m))
		case "none":
			// skip
		default:
			sb.WriteString(fmt.Sprintf("  %s:\n", p))
			if ak != "" {
				sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", ak))
			}
			if bu != "" {
				sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", bu))
			}
			m := model
			if m == "" {
				m = "default"
			}
			sb.WriteString(fmt.Sprintf("    default_model: \"%s\"\n", m))
		}
	}

	writeProvider(provider, apiKey, baseURL, selectedModel)
	if secondaryProvider != "none" && secondaryProvider != provider {
		writeProvider(secondaryProvider, secondaryAPIKey, secondaryBaseURL, "default")
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
	provName := provider
	if provName == "azure" {
		provName = "azure_openai"
	}
	m := selectedModel
	if m == "" {
		m = "default"
	}
	sb.WriteString(fmt.Sprintf("  default: \"%s/%s\"\n", provName, m))
	if secondaryProvider != "none" {
		secName := secondaryProvider
		if secName == "azure" {
			secName = "azure_openai"
		}
		sb.WriteString(fmt.Sprintf("  fallback: \"%s/default\"\n", secName))
	}

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
