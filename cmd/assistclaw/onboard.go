package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type provEntry struct {
	provider     string
	apiKey       string
	baseURL      string
	apiVersion   string
	awsRegion    string
	awsProfile   string
	awsAccessKey string
	awsSecretKey string
	vertexProj   string
	vertexLoc    string
	vertexCreds  string
	model        string
}

type embedEntry struct {
	provider string
	apiKey   string
	baseURL  string
	model    string
}

func onboardCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Run the interactive first-time setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := gf.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			shouldStart, err := runOnboarding(path)
			if err != nil {
				return err
			}
			if shouldStart {
				fmt.Println("\n🚀 Starting agent as requested...")
				return runAgent(gf, path, "", "", "", true, false)
			}
			return nil
		},
	}
}

// collectProvider guides the user through selecting and configuring an LLM provider.
func collectProvider(theme *huh.Theme, providerType string, isPrimary bool, initial provEntry) (provEntry, error) {
	entry := initial
	var providerOptions []huh.Option[string]

	if isPrimary {
		providerOptions = []huh.Option[string]{
			huh.NewOption("Anthropic (Recommended)", "anthropic"),
			huh.NewOption("OpenAI", "openai"),
			huh.NewOption("Ollama (Local / Free)", "ollama"),
			huh.NewOption("AWS Bedrock", "bedrock"),
			huh.NewOption("vLLM (Local / Custom)", "vllm"),
			huh.NewOption("LM Studio (Local)", "lmstudio"),
			huh.NewOption("Groq", "groq"),
			huh.NewOption("Mistral", "mistral"),
			huh.NewOption("Azure OpenAI", "azure"),
			huh.NewOption("DeepSeek", "deepseek"),
			huh.NewOption("Perplexity", "perplexity"),
			huh.NewOption("Google Vertex AI", "vertex"),
		}
	} else {
		providerOptions = []huh.Option[string]{
			huh.NewOption("Ollama (Local / Free)", "ollama"),
			huh.NewOption("OpenAI", "openai"),
			huh.NewOption("Groq (Super Fast)", "groq"),
			huh.NewOption("Anthropic", "anthropic"),
			huh.NewOption("Mistral", "mistral"),
			huh.NewOption("OpenRouter", "openrouter"),
			huh.NewOption("Azure OpenAI", "azure"),
			huh.NewOption("DeepSeek", "deepseek"),
			huh.NewOption("Perplexity", "perplexity"),
			huh.NewOption("AWS Bedrock", "bedrock"),
			huh.NewOption("Google Vertex AI", "vertex"),
			huh.NewOption("vLLM (Local / Custom)", "vllm"),
			huh.NewOption("LM Studio (Local)", "lmstudio"),
		}
	}

	title := fmt.Sprintf("Which %s AI provider would you like to use?", providerType)
	formProvider := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(providerOptions...).
				Value(&entry.provider),
		),
	).WithTheme(theme)

	if err := formProvider.Run(); err != nil {
		return provEntry{}, fmt.Errorf("onboarding interrupted")
	}

	var fields []huh.Field
	needsAPIKey := map[string]bool{
		"anthropic":  true,
		"openai":     true,
		"groq":       true,
		"mistral":    true,
		"openrouter": true,
		"azure":      true,
		"deepseek":   true,
		"perplexity": true,
		"voyage":     true,
	}

	needsBaseURL := map[string]string{
		"ollama":   "http://localhost:11434",
		"vllm":     "http://localhost:8000/v1",
		"lmstudio": "http://localhost:1234/v1",
		"azure":    "https://YOUR_RESOURCE_NAME.openai.azure.com",
	}

	if needsAPIKey[entry.provider] {
		fields = append(fields, huh.NewInput().
			Title("Enter API Key").
			Description("Stored safely in your local configuration.").
			Password(true).
			Value(&entry.apiKey))
	}

	if defaultURL, ok := needsBaseURL[entry.provider]; ok {
		fields = append(fields, huh.NewInput().
			Title(fmt.Sprintf("Enter Base URL (Default: %s)", defaultURL)).
			Value(&entry.baseURL))
	}

	if entry.provider == "azure" {
		fields = append(fields, huh.NewInput().
			Title("Enter Azure API Version (e.g., 2024-02-15-preview)").
			Value(&entry.apiVersion))
	}

	if entry.provider == "bedrock" {
		var bedrockAuthMode string
		entry.awsRegion = "us-east-1"
		formBedrockAuth := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("AWS Bedrock Authentication").
					Description("Tip: Bearer Tokens are currently most stable in us-east-1. IAM keys are recommended for other regions.").
					Options(
						huh.NewOption("Direct IAM Keys (AccessKeyID/SecretKey)", "iam"),
						huh.NewOption("AWS Named Profile (~/.aws/credentials)", "profile"),
						huh.NewOption("Native Bedrock API Key (Bearer Token)", "api_key"),
					).
					Value(&bedrockAuthMode),
			),
		).WithTheme(theme)
		if err := formBedrockAuth.Run(); err != nil {
			return provEntry{}, fmt.Errorf("onboarding interrupted")
		}

		fields = append(fields, huh.NewInput().Title("AWS Region").Value(&entry.awsRegion))

		switch bedrockAuthMode {
		case "iam":
			fields = append(fields,
				huh.NewInput().Title("AWS Access Key ID").Value(&entry.awsAccessKey),
				huh.NewInput().Title("AWS Secret Access Key").Password(true).Value(&entry.awsSecretKey),
			)
		case "profile":
			entry.awsProfile = "default"
			fields = append(fields, huh.NewInput().Title("AWS Profile").Value(&entry.awsProfile))
		case "api_key":
			fields = append(fields, huh.NewInput().Title("Bedrock API Key").Password(true).Value(&entry.apiKey))
		}
	}

	if entry.provider == "vertex" {
		fields = append(fields,
			huh.NewInput().Title("GCP Project ID").Value(&entry.vertexProj),
			huh.NewInput().Title("GCP Location (e.g. us-central1)").Value(&entry.vertexLoc),
			huh.NewInput().Title("Service Account JSON Path (Optional)").Value(&entry.vertexCreds),
		)
	}

	if len(fields) > 0 {
		formDetails := huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme)
		if err := formDetails.Run(); err != nil {
			return provEntry{}, fmt.Errorf("onboarding interrupted")
		}
	}

	modelChoices := map[string][]huh.Option[string]{
		"anthropic": {
			huh.NewOption("Claude 3.7 Sonnet (Latest)", "claude-3-7-sonnet-20250219"),
			huh.NewOption("Claude 3.5 Sonnet (Classic)", "claude-3-5-sonnet-20241022"),
			huh.NewOption("Claude 3.5 Haiku (Fast)", "claude-3-5-haiku-20241022"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"openai": {
			huh.NewOption("GPT-4o (Smartest)", "gpt-4o"),
			huh.NewOption("GPT-4o-mini (Efficient)", "gpt-4o-mini"),
			huh.NewOption("o3-mini (Reasoning)", "o3-mini"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"ollama": {
			huh.NewOption("Llama 3.2 (3B)", "llama3.2"),
			huh.NewOption("Mistral (7B)", "mistral"),
			huh.NewOption("DeepSeek-R1 (70B Distill)", "deepseek-r1"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"bedrock": {
			huh.NewOption("Claude 3.5 Sonnet v2", "anthropic.claude-3-5-sonnet-20241022-v2:0"),
			huh.NewOption("Claude 3.5 Haiku", "anthropic.claude-3-5-haiku-20241022-v1:0"),
			huh.NewOption("Llama 3.3 70B", "meta.llama3-3-70b-instruct-v1:0"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"groq": {
			huh.NewOption("Llama 3.3 70B Versatile", "llama-3.3-70b-versatile"),
			huh.NewOption("Mixtral 8x7B", "mixtral-8x7b-32768"),
			huh.NewOption("DeepSeek-R1 70B", "deepseek-r1-distill-llama-70b"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"deepseek": {
			huh.NewOption("DeepSeek Chat", "deepseek-chat"),
			huh.NewOption("DeepSeek Reasoner (R1)", "deepseek-reasoner"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"perplexity": {
			huh.NewOption("Sonar Reasoning Pro", "sonar-reasoning-pro"),
			huh.NewOption("Sonar Pro", "sonar-pro"),
			huh.NewOption("Sonar Reasoning", "sonar-reasoning"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"vertex": {
			huh.NewOption("Gemini 1.5 Pro", "gemini-1.5-pro"),
			huh.NewOption("Gemini 1.5 Flash", "gemini-1.5-flash"),
			huh.NewOption("Gemini 2.0 Flash Exp", "gemini-2.0-flash-exp"),
			huh.NewOption("Other / Custom...", "custom"),
		},
	}

	if opts, ok := modelChoices[entry.provider]; ok {
		formModel := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Model").
					Options(opts...).
					Value(&entry.model),
			),
		).WithTheme(theme)
		if err := formModel.Run(); err != nil {
			return provEntry{}, fmt.Errorf("onboarding interrupted")
		}

		if entry.model == "custom" {
			formCustom := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Enter Custom Model ID").
						Value(&entry.model),
				),
			).WithTheme(theme)
			if err := formCustom.Run(); err != nil {
				return provEntry{}, fmt.Errorf("onboarding interrupted")
			}
		}
	} else {
		formModel := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter Model ID").
					Value(&entry.model),
			),
		).WithTheme(theme)
		if err := formModel.Run(); err != nil {
			return provEntry{}, fmt.Errorf("onboarding interrupted")
		}
	}

	return entry, nil
}

// runOnboarding leads the user through the setup process.
// Returns (true, nil) if the agent should be started immediately.
func runOnboarding(configPath string) (bool, error) {
	var (
		primary          provEntry
		secondary        provEntry
		embed            embedEntry
		gwMode           string
		gwPort           int    = 18790
		gwHost           string = "127.0.0.1"
		gwToken          string
		tsMode           string = "off"
		selectedChannels []string
		tgBotToken       string
		waSessionID      string
		selectedSkills   []string
		codingModel      string
		visionModel      string
		tpl              string
	)

	theme := huh.ThemeBase()
	theme.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	theme.Focused.SelectedOption = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	theme.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("  Welcome to AssistClaw 🐾"))
	fmt.Println(lipgloss.NewStyle().Faint(true).Render("  Let's configure your autonomous agent environment."))

	// Load existing config if available to pre-populate defaults
	var existing *config.Config
	if _, err := os.Stat(configPath); err == nil {
		if c, err := config.Load(configPath); err == nil {
			existing = c
		}
	}

	if existing != nil {
		// Pre-populate Primary
		defProv := ""
		if existing.Routing.Default != "" {
			parts := strings.Split(existing.Routing.Default, "/")
			if len(parts) > 0 {
				defProv = parts[0]
				if defProv == "azure_openai" {
					defProv = "azure"
				}
			}
		}

		if defProv != "" {
			primary.provider = defProv
			switch defProv {
			case "anthropic":
				if existing.Providers.Anthropic != nil {
					primary.apiKey = existing.Providers.Anthropic.APIKey
					primary.baseURL = existing.Providers.Anthropic.BaseURL
					primary.model = existing.Providers.Anthropic.DefaultModel
				}
			case "openai":
				if existing.Providers.OpenAI != nil {
					primary.apiKey = existing.Providers.OpenAI.APIKey
					primary.baseURL = existing.Providers.OpenAI.BaseURL
					primary.model = existing.Providers.OpenAI.DefaultModel
				}
			case "azure":
				if existing.Providers.AzureOpenAI != nil {
					primary.apiKey = existing.Providers.AzureOpenAI.APIKey
					primary.baseURL = existing.Providers.AzureOpenAI.BaseURL
					primary.apiVersion = existing.Providers.AzureOpenAI.APIVersion
					primary.model = existing.Providers.AzureOpenAI.DefaultModel
				}
			case "ollama":
				if existing.Providers.Ollama != nil {
					primary.baseURL = existing.Providers.Ollama.BaseURL
					primary.model = existing.Providers.Ollama.DefaultModel
				}
			case "vllm":
				if existing.Providers.VLLM != nil {
					primary.baseURL = existing.Providers.VLLM.BaseURL
					primary.apiKey = existing.Providers.VLLM.APIKey
					primary.model = existing.Providers.VLLM.DefaultModel
				}
			case "lmstudio":
				if existing.Providers.LMStudio != nil {
					primary.baseURL = existing.Providers.LMStudio.BaseURL
					primary.model = existing.Providers.LMStudio.DefaultModel
				}
			case "bedrock":
				if existing.Providers.Bedrock != nil {
					primary.awsRegion = existing.Providers.Bedrock.Region
					primary.awsProfile = existing.Providers.Bedrock.Profile
					primary.awsAccessKey = existing.Providers.Bedrock.AccessKeyID
					primary.awsSecretKey = existing.Providers.Bedrock.SecretAccessKey
					primary.apiKey = existing.Providers.Bedrock.APIKey
					primary.model = existing.Providers.Bedrock.DefaultModel
				}
			case "groq":
				if existing.Providers.Groq != nil {
					primary.apiKey = existing.Providers.Groq.APIKey
					primary.model = existing.Providers.Groq.DefaultModel
				}
			case "mistral":
				if existing.Providers.Mistral != nil {
					primary.apiKey = existing.Providers.Mistral.APIKey
					primary.model = existing.Providers.Mistral.DefaultModel
				}
			case "openrouter":
				if existing.Providers.OpenRouter != nil {
					primary.apiKey = existing.Providers.OpenRouter.APIKey
					primary.model = existing.Providers.OpenRouter.DefaultModel
				}
			case "deepseek":
				if existing.Providers.DeepSeek != nil {
					primary.apiKey = existing.Providers.DeepSeek.APIKey
					primary.model = existing.Providers.DeepSeek.DefaultModel
				}
			case "perplexity":
				if existing.Providers.Perplexity != nil {
					primary.apiKey = existing.Providers.Perplexity.APIKey
					primary.model = existing.Providers.Perplexity.DefaultModel
				}
			case "vertex":
				if existing.Providers.Vertex != nil {
					primary.vertexProj = existing.Providers.Vertex.ProjectID
					primary.vertexLoc = existing.Providers.Vertex.Location
					primary.vertexCreds = existing.Providers.Vertex.Credentials
					primary.model = existing.Providers.Vertex.DefaultModel
				}
			}
		}

		// Pre-populate Secondary
		if existing.Routing.Fallback != "" {
			parts := strings.Split(existing.Routing.Fallback, "/")
			if len(parts) > 0 {
				sProv := parts[0]
				if sProv == "azure_openai" {
					sProv = "azure"
				}
				secondary.provider = sProv
				switch sProv {
				case "anthropic":
					if existing.Providers.Anthropic != nil {
						secondary.apiKey = existing.Providers.Anthropic.APIKey
						secondary.baseURL = existing.Providers.Anthropic.BaseURL
						secondary.model = existing.Providers.Anthropic.DefaultModel
					}
				case "openai":
					if existing.Providers.OpenAI != nil {
						secondary.apiKey = existing.Providers.OpenAI.APIKey
						secondary.baseURL = existing.Providers.OpenAI.BaseURL
						secondary.model = existing.Providers.OpenAI.DefaultModel
					}
				case "azure":
					if existing.Providers.AzureOpenAI != nil {
						secondary.apiKey = existing.Providers.AzureOpenAI.APIKey
						secondary.baseURL = existing.Providers.AzureOpenAI.BaseURL
						secondary.apiVersion = existing.Providers.AzureOpenAI.APIVersion
						secondary.model = existing.Providers.AzureOpenAI.DefaultModel
					}
				case "ollama":
					if existing.Providers.Ollama != nil {
						secondary.baseURL = existing.Providers.Ollama.BaseURL
						secondary.model = existing.Providers.Ollama.DefaultModel
					}
				case "vllm":
					if existing.Providers.VLLM != nil {
						secondary.baseURL = existing.Providers.VLLM.BaseURL
						secondary.apiKey = existing.Providers.VLLM.APIKey
						secondary.model = existing.Providers.VLLM.DefaultModel
					}
				case "lmstudio":
					if existing.Providers.LMStudio != nil {
						secondary.baseURL = existing.Providers.LMStudio.BaseURL
						secondary.model = existing.Providers.LMStudio.DefaultModel
					}
				case "bedrock":
					if existing.Providers.Bedrock != nil {
						secondary.awsRegion = existing.Providers.Bedrock.Region
						secondary.awsProfile = existing.Providers.Bedrock.Profile
						secondary.awsAccessKey = existing.Providers.Bedrock.AccessKeyID
						secondary.awsSecretKey = existing.Providers.Bedrock.SecretAccessKey
						secondary.apiKey = existing.Providers.Bedrock.APIKey
						secondary.model = existing.Providers.Bedrock.DefaultModel
					}
				case "groq":
					if existing.Providers.Groq != nil {
						secondary.apiKey = existing.Providers.Groq.APIKey
						secondary.model = existing.Providers.Groq.DefaultModel
					}
				case "mistral":
					if existing.Providers.Mistral != nil {
						secondary.apiKey = existing.Providers.Mistral.APIKey
						secondary.model = existing.Providers.Mistral.DefaultModel
					}
				case "openrouter":
					if existing.Providers.OpenRouter != nil {
						secondary.apiKey = existing.Providers.OpenRouter.APIKey
						secondary.model = existing.Providers.OpenRouter.DefaultModel
					}
				case "deepseek":
					if existing.Providers.DeepSeek != nil {
						secondary.apiKey = existing.Providers.DeepSeek.APIKey
						secondary.model = existing.Providers.DeepSeek.DefaultModel
					}
				case "perplexity":
					if existing.Providers.Perplexity != nil {
						secondary.apiKey = existing.Providers.Perplexity.APIKey
						secondary.model = existing.Providers.Perplexity.DefaultModel
					}
				case "vertex":
					if existing.Providers.Vertex != nil {
						secondary.vertexProj = existing.Providers.Vertex.ProjectID
						secondary.vertexLoc = existing.Providers.Vertex.Location
						secondary.vertexCreds = existing.Providers.Vertex.Credentials
						secondary.model = existing.Providers.Vertex.DefaultModel
					}
				}
			}
		}

		// Pre-populate Embeddings
		if len(existing.Embeddings.Priority) > 0 {
			eName := existing.Embeddings.Priority[0]
			if eName == "azure_openai" {
				eName = "azure"
			}
			embed.provider = eName
			switch eName {
			case "openai":
				if existing.Embeddings.OpenAI != nil {
					embed.apiKey = existing.Embeddings.OpenAI.APIKey
					embed.baseURL = existing.Embeddings.OpenAI.BaseURL
					embed.model = existing.Embeddings.OpenAI.DefaultModel
				}
			case "azure":
				if existing.Embeddings.AzureOpenAI != nil {
					embed.apiKey = existing.Embeddings.AzureOpenAI.APIKey
					embed.baseURL = existing.Embeddings.AzureOpenAI.BaseURL
					embed.model = existing.Embeddings.AzureOpenAI.DefaultModel
				}
			case "ollama":
				if existing.Embeddings.OllamaEmbed != nil {
					embed.baseURL = existing.Embeddings.OllamaEmbed.BaseURL
					embed.model = existing.Embeddings.OllamaEmbed.DefaultModel
				}
			case "bedrock":
				if existing.Embeddings.Bedrock != nil {
					embed.apiKey = existing.Embeddings.Bedrock.APIKey // If using bearer token
					embed.model = existing.Embeddings.Bedrock.DefaultModel
				}
			case "cohere":
				if existing.Embeddings.Cohere != nil {
					embed.apiKey = existing.Embeddings.Cohere.APIKey
					embed.model = existing.Embeddings.Cohere.DefaultModel
				}
			case "google":
				if existing.Embeddings.Google != nil {
					embed.apiKey = existing.Embeddings.Google.APIKey
					embed.model = existing.Embeddings.Google.DefaultModel
				}
			case "voyage":
				if existing.Embeddings.Voyage != nil {
					embed.apiKey = existing.Embeddings.Voyage.APIKey
					embed.model = existing.Embeddings.Voyage.DefaultModel
				}
			case "mistral":
				if existing.Embeddings.Mistral != nil {
					embed.apiKey = existing.Embeddings.Mistral.APIKey
					embed.model = existing.Embeddings.Mistral.DefaultModel
				}
			case "vertex":
				if existing.Embeddings.Vertex != nil {
					embed.model = existing.Embeddings.Vertex.DefaultModel
				}
			}
		}

		// Pre-populate Gateway & Routing
		gwMode = existing.Gateway.Bind
		if existing.Gateway.Host != "" {
			gwHost = existing.Gateway.Host
		}
		if existing.Gateway.Port != 0 {
			gwPort = existing.Gateway.Port
		}
		gwToken = existing.Gateway.Token
		tsMode = existing.Gateway.Tailscale.Mode
		for _, r := range existing.Routing.Rules {
			if r.Task == "coding" {
				codingModel = r.Model
			}
			if r.Task == "vision" {
				visionModel = r.Model
			}
		}

		// Pre-populate Channels
		if existing.Channels.Telegram != nil {
			selectedChannels = append(selectedChannels, "telegram")
			tgBotToken = existing.Channels.Telegram.BotToken
		}
		if existing.Channels.WhatsApp != nil {
			selectedChannels = append(selectedChannels, "whatsapp")
			waSessionID = existing.Channels.WhatsApp.SessionID
		}
		if existing.Channels.Discord != nil {
			selectedChannels = append(selectedChannels, "discord")
		}
		if existing.Channels.Slack != nil {
			selectedChannels = append(selectedChannels, "slack")
		}

		// Pre-populate Skills
		selectedSkills = existing.Agent.EnabledSkills
	}

	var err error
	primary, err = collectProvider(theme, "primary", true, primary)
	if err != nil {
		return false, err
	}

	secChoice := "none"
	if secondary.provider != "" && secondary.provider != "none" {
		secChoice = "configure"
	}

	formSecondaryChoice := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Secondary / Fallback Provider?").
				Description("Pick a second model for high availability or specific tasks.").
				Options(
					huh.NewOption("None", "none"),
					huh.NewOption("Choose a secondary provider", "configure"),
				).
				Value(&secChoice),
		),
	).WithTheme(theme)
	if err := formSecondaryChoice.Run(); err != nil {
		return false, fmt.Errorf("onboarding interrupted")
	}

	if secChoice == "configure" {
		secondary, err = collectProvider(theme, "secondary", false, secondary)
		if err != nil {
			return false, err
		}
	} else {
		secondary.provider = "none"
	}

	formRouting := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Advanced Routing: Coding").
				Options(
					huh.NewOption("Use Default", "default"),
					huh.NewOption("Claude 3.5 Sonnet", "anthropic/claude-3-5-sonnet-20241022"),
					huh.NewOption("GPT-4o", "openai/gpt-4o"),
					huh.NewOption("DeepSeek-R1 (Local)", "ollama/deepseek-r1"),
				).
				Value(&codingModel),
			huh.NewSelect[string]().
				Title("Advanced Routing: Vision").
				Options(
					huh.NewOption("Use Default", "default"),
					huh.NewOption("Claude 3.5 Sonnet", "anthropic/claude-3-5-sonnet-20241022"),
					huh.NewOption("GPT-4o", "openai/gpt-4o"),
				).
				Value(&visionModel),
		),
	).WithTheme(theme)
	if err := formRouting.Run(); err != nil {
		return false, fmt.Errorf("onboarding interrupted")
	}

	// Embedding selection
	formEmbed := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Embedding Provider").
				Description("Used for Semantic Memory (local learning).").
				Options(
					huh.NewOption("OpenAI (Recommended)", "openai"),
					huh.NewOption("Azure OpenAI", "azure"),
					huh.NewOption("Ollama (Local)", "ollama"),
					huh.NewOption("AWS Bedrock", "bedrock"),
					huh.NewOption("Cohere", "cohere"),
					huh.NewOption("Google Generative AI", "google"),
					huh.NewOption("Voyage AI", "voyage"),
					huh.NewOption("Mistral Native", "mistral"),
					huh.NewOption("Google Vertex AI", "vertex"),
				).
				Value(&embed.provider),
		),
	).WithTheme(theme)
	if err := formEmbed.Run(); err != nil {
		return false, fmt.Errorf("onboarding interrupted")
	}

	var embedFields []huh.Field
	if embed.provider == "ollama" {
		embed.baseURL = "http://localhost:11434"
		embedFields = append(embedFields, huh.NewInput().Title("Ollama Base URL").Value(&embed.baseURL))
	} else if embed.provider == "azure" {
		embedFields = append(embedFields,
			huh.NewInput().Title("Azure Endpoint").Value(&embed.baseURL),
			huh.NewInput().Title("Azure API Key").Password(true).Value(&embed.apiKey),
		)
	} else if embed.provider != "bedrock" {
		embedFields = append(embedFields, huh.NewInput().
			Title(fmt.Sprintf("%s API Key (Embeddings)", embed.provider)).
			Password(true).
			Value(&embed.apiKey))
	}

	embedModels := map[string][]huh.Option[string]{
		"openai": {huh.NewOption("text-embedding-3-small", "text-embedding-3-small"), huh.NewOption("text-embedding-3-large", "text-embedding-3-large")},
		"azure":  {huh.NewOption("text-embedding-3-small", "text-embedding-3-small"), huh.NewOption("text-embedding-3-large", "text-embedding-3-large")},
		"ollama": {huh.NewOption("nomic-embed-text", "nomic-embed-text"), huh.NewOption("mxbai-embed-large", "mxbai-embed-large")},
		"bedrock": {
			huh.NewOption("Titan Text Embed v2", "amazon.titan-embed-text-v2:0"),
			huh.NewOption("Titan Text Embed v1", "amazon.titan-embed-text-v1"),
			huh.NewOption("Cohere English v3", "cohere.embed-english-v3"),
		},
		"cohere":  {huh.NewOption("embed-v4.0", "embed-v4.0")},
		"google":  {huh.NewOption("text-embedding-004", "text-embedding-004")},
		"voyage":  {huh.NewOption("voyage-3", "voyage-3"), huh.NewOption("voyage-3-lite", "voyage-3-lite")},
		"mistral": {huh.NewOption("mistral-embed", "mistral-embed")},
		"vertex":  {huh.NewOption("text-embedding-004", "text-embedding-004"), huh.NewOption("text-multilingual-embedding-002", "text-multilingual-embedding-002")},
	}
	embedFields = append(embedFields, huh.NewSelect[string]().Title("Embedding Model").Options(embedModels[embed.provider]...).Value(&embed.model))

	if len(embedFields) > 0 {
		formEmbedDetail := huh.NewForm(huh.NewGroup(embedFields...)).WithTheme(theme)
		if err := formEmbedDetail.Run(); err != nil {
			return false, fmt.Errorf("onboarding interrupted")
		}
	}

	// Phases 3-5: Gateway, Channels, Skills (simplified for brevity)
	var gatewayFields []huh.Field
	gatewayFields = append(gatewayFields,
		huh.NewSelect[string]().
			Title("Remote Access Mode").
			Options(
				huh.NewOption("Local Only (127.0.0.1)", "loopback"),
				huh.NewOption("Local Network (LAN - 0.0.0.0)", "lan"),
				huh.NewOption("Tailscale (Secure VPN)", "tailscale"),
			).
			Value(&gwMode),
		huh.NewInput().Title("Gateway Host").Value(&gwHost),
		func() huh.Field {
			portStr := fmt.Sprint(gwPort)
			return huh.NewInput().Title("Gateway Port").Value(&portStr).Validate(func(s string) error {
				var p int
				_, err := fmt.Sscan(s, &p)
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("invalid port")
				}
				gwPort = p
				return nil
			})
		}(),
		huh.NewInput().Title("Security Token").Description("Password to protect your Gateway API. Leave empty for none.").Value(&gwToken),
	)

	formGateway := huh.NewForm(huh.NewGroup(gatewayFields...)).WithTheme(theme)
	_ = formGateway.Run()

	if gwMode == "tailscale" {
		formTS := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Tailscale Mode").
					Options(
						huh.NewOption("Off", "off"),
						huh.NewOption("Serve (Local Tailnet)", "serve"),
						huh.NewOption("Funnel (Public Internet via Tailscale)", "funnel"),
					).
					Value(&tsMode),
			),
		).WithTheme(theme)
		_ = formTS.Run()
	}

	formChannels := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Messaging Channels").
				Options(
					huh.NewOption("Telegram", "telegram"),
					huh.NewOption("Discord", "discord"),
					huh.NewOption("Slack", "slack"),
					huh.NewOption("WhatsApp", "whatsapp"),
				).
				Value(&selectedChannels),
		),
	).WithTheme(theme)
	_ = formChannels.Run()

	for _, ch := range selectedChannels {
		switch ch {
		case "telegram":
			_ = huh.NewForm(huh.NewGroup(huh.NewInput().Title("Telegram Token").Value(&tgBotToken))).Run()
		case "whatsapp":
			waSessionID = "personal"
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("\n--- WhatsApp Integration ---"))
			fmt.Println("AssistClaw acts as a standalone WhatsApp account.")
			fmt.Println("You will need to scan a QR code using 'Linked Devices' on your phone.")

			_ = huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("WhatsApp Session ID").
					Description("A name for this session (e.g. 'personal')").
					Value(&waSessionID),
			)).WithTheme(theme).Run()
		}
	}

	// Dynamic Skill Discovery
	var skillOptions []huh.Option[string]
	skillReg := skills.NewRegistry()

	// 1. Check current directory
	availableSkills, _ := skillReg.Discover("./skills")
	// 2. Check default state directory (~/.assistclaw/skills)
	if home, err := os.UserHomeDir(); err == nil {
		homeSkills := filepath.Join(home, ".assistclaw", "skills")
		if homeSkills != "./skills" { // avoid double-counting if we are IN ~/.assistclaw
			more, _ := skillReg.Discover(homeSkills)
			availableSkills = append(availableSkills, more...)
		}
	}

	// Deduplicate by name
	seen := make(map[string]bool)
	var finalSkills []skills.SkillInfo
	for _, s := range availableSkills {
		if !seen[s.Name] {
			seen[s.Name] = true
			finalSkills = append(finalSkills, s)
		}
	}

	for _, s := range finalSkills {
		label := s.Name
		if s.Emoji != "" {
			label = s.Emoji + " " + s.Name
		}
		if !s.Eligible {
			label = "❗ " + label + " (Missing: " + strings.Join(s.Missing, ", ") + ")"
		}
		skillOptions = append(skillOptions, huh.NewOption(label, s.Name))
	}

	// Fallback if no skills found on disk
	if len(skillOptions) == 0 {
		skillOptions = []huh.Option[string]{
			huh.NewOption("Browser Control", "browser"),
			huh.NewOption("System Admin", "sysadmin"),
			huh.NewOption("Development Assistant", "dev"),
		}
	}

	formSkills := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Enable Skills").
				Description("Choose specialized capabilities for your agent.").
				Options(skillOptions...).
				Value(&selectedSkills),
		),
	).WithTheme(theme)
	_ = formSkills.Run()

	// Handle installation for missing dependencies
	for _, name := range selectedSkills {
		s, ok := skillReg.Get(name)
		if !ok {
			// Try to load it fully if discovered but not yet in registry
			// Actually Discover doesn't put them in the registry, it just returns info.
			// Let's reload everything from expected dirs to be sure.
			_ = skillReg.LoadAll(context.Background(), "./skills")
			if home, err := os.UserHomeDir(); err == nil {
				_ = skillReg.LoadAll(context.Background(), filepath.Join(home, ".assistclaw", "skills"))
			}
			s, ok = skillReg.Get(name)
		}

		if ok {
			met, missing := skillReg.CheckRequirements(s)
			if !met && len(s.Metadata.OpenClaw.Install) > 0 {
				var confirmInstall bool
				huh.NewForm(huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Install dependencies for %s (%s)?", s.Name, strings.Join(missing, ", "))).
						Description("This will run brew or go install as specified in the skill metadata.").
						Value(&confirmInstall),
				)).WithTheme(theme).Run()

				if confirmInstall {
					if err := skillReg.InstallDependency(context.Background(), s); err != nil {
						fmt.Printf("Skill installation failed: %v\n", err)
					} else {
						fmt.Printf("✔ Successfully installed dependencies for %s\n", s.Name)
					}
				}
			}
		}
	}

	// Build the YAML
	var sb strings.Builder
	sb.WriteString("# AssistClaw Configuration\nversion: 1\n\n")

	sb.WriteString("gateway:\n")
	sb.WriteString(fmt.Sprintf("  host: \"%s\"\n  port: %d\n  bind: \"%s\"\n", gwHost, gwPort, gwMode))
	if gwToken != "" {
		sb.WriteString(fmt.Sprintf("  token: \"%s\"\n", gwToken))
	}
	if gwMode == "tailscale" {
		sb.WriteString("  tailscale:\n")
		sb.WriteString(fmt.Sprintf("    mode: \"%s\"\n", tsMode))
	}
	sb.WriteString("\n")

	sb.WriteString("agent:\n  max_iterations: 64\n")
	if len(selectedSkills) > 0 {
		sb.WriteString("  enabled_skills:\n")
		for _, s := range selectedSkills {
			sb.WriteString(fmt.Sprintf("    - \"%s\"\n", s))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("providers:\n")
	writeProv := func(e provEntry) {
		name := e.provider
		if name == "azure" {
			name = "azure_openai"
		}
		sb.WriteString(fmt.Sprintf("  %s:\n", name))
		if e.apiKey != "" {
			sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", e.apiKey))
		}
		if e.baseURL != "" {
			sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", e.baseURL))
		}
		if e.awsRegion != "" {
			sb.WriteString(fmt.Sprintf("    region: \"%s\"\n", e.awsRegion))
		}
		if e.awsAccessKey != "" {
			sb.WriteString(fmt.Sprintf("    access_key_id: \"%s\"\n", e.awsAccessKey))
			sb.WriteString(fmt.Sprintf("    secret_access_key: \"%s\"\n", e.awsSecretKey))
		}
		if e.awsProfile != "" {
			sb.WriteString(fmt.Sprintf("    profile: \"%s\"\n", e.awsProfile))
		}
		if e.vertexProj != "" {
			sb.WriteString(fmt.Sprintf("    project_id: \"%s\"\n", e.vertexProj))
		}
		if e.vertexLoc != "" {
			sb.WriteString(fmt.Sprintf("    location: \"%s\"\n", e.vertexLoc))
		}
		if e.vertexCreds != "" {
			sb.WriteString(fmt.Sprintf("    credentials: \"%s\"\n", e.vertexCreds))
		}
		sb.WriteString(fmt.Sprintf("    default_model: \"%s\"\n", e.model))
	}

	writeProv(primary)
	if secondary.provider != "" && secondary.provider != "none" {
		writeProv(secondary)
	}
	sb.WriteString("\n")

	sb.WriteString("embeddings:\n")
	sb.WriteString(fmt.Sprintf("  priority:\n    - \"%s\"\n", embed.provider))
	writeEmbed := func(e embedEntry) {
		name := e.provider
		if name == "azure" {
			name = "azure_openai"
		}
		sb.WriteString(fmt.Sprintf("  %s:\n", name))
		if e.apiKey != "" {
			sb.WriteString(fmt.Sprintf("    api_key: \"%s\"\n", e.apiKey))
		}
		if e.baseURL != "" {
			sb.WriteString(fmt.Sprintf("    base_url: \"%s\"\n", e.baseURL))
		}
		sb.WriteString(fmt.Sprintf("    model: \"%s\"\n", e.model))
	}
	writeEmbed(embed)
	sb.WriteString("\n")

	if len(selectedChannels) > 0 {
		sb.WriteString("channels:\n")
		for _, ch := range selectedChannels {
			sb.WriteString(fmt.Sprintf("  %s:\n", ch))
			if ch == "telegram" {
				sb.WriteString(fmt.Sprintf("    bot_token: \"%s\"\n", tgBotToken))
			}
			if ch == "whatsapp" {
				sb.WriteString(fmt.Sprintf("    session_id: \"%s\"\n", waSessionID))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("routing:\n")
	pName := primary.provider
	if pName == "azure" {
		pName = "azure_openai"
	}
	sb.WriteString(fmt.Sprintf("  default: \"%s/%s\"\n", pName, primary.model))
	if secondary.provider != "" && secondary.provider != "none" {
		sName := secondary.provider
		if sName == "azure" {
			sName = "azure_openai"
		}
		sb.WriteString(fmt.Sprintf("  fallback: \"%s/%s\"\n", sName, secondary.model))
	}
	if codingModel != "default" && codingModel != "" {
		sb.WriteString("  rules:\n")
		sb.WriteString(fmt.Sprintf("    - task: \"coding\"\n      model: \"%s\"\n", codingModel))
	}
	if visionModel != "default" && visionModel != "" {
		if codingModel == "default" || codingModel == "" {
			sb.WriteString("  rules:\n")
		}
		sb.WriteString(fmt.Sprintf("    - task: \"vision\"\n      model: \"%s\"\n", visionModel))
	}

	tpl = sb.String()
	_ = os.MkdirAll(filepath.Dir(configPath), 0o755)
	_ = os.WriteFile(configPath, []byte(tpl), 0o600)

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Configuration saved!"))

	fmt.Println("\nNext Steps:")
	fmt.Println("1. Run 'assistclaw agent' to start your assistant manually.")
	fmt.Println("2. Scan the QR code if you enabled WhatsApp.")

	var startAgent bool
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Start your assistant now in background mode?").
				Description("This will start the Gateway and your messaging channels.").
				Value(&startAgent),
		),
	).Run()
	if err != nil {
		return false, nil // User cancelled the confirm, but config is saved
	}

	return startAgent, nil
}
