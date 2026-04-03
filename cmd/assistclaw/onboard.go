package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/assistclaw/assistclaw/internal/channels/whatsapp"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/provider/bedrock"
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

func inferBedrockAuthMode(e provEntry) string {
	if strings.TrimSpace(e.apiKey) != "" {
		return "api_key"
	}
	if strings.TrimSpace(e.awsProfile) != "" {
		return "profile"
	}
	if strings.TrimSpace(e.awsAccessKey) != "" {
		return "iam"
	}
	return "profile"
}

// ensureSelectValue prepends a select option when the saved value is not in the list (e.g. custom model IDs).
func ensureSelectValue(opts []huh.Option[string], current, prefix string) []huh.Option[string] {
	current = strings.TrimSpace(current)
	if current == "" || current == "custom" {
		return opts
	}
	for _, o := range opts {
		if o.Value == current {
			return opts
		}
	}
	label := current
	if len(label) > 72 {
		label = label[:69] + "…"
	}
	return append([]huh.Option[string]{huh.NewOption(prefix + label, current)}, opts...)
}

func normalizeGatewayBind(bind string) string {
	switch strings.TrimSpace(bind) {
	case "loopback", "127.0.0.1", "":
		return "loopback"
	case "lan", "0.0.0.0":
		return "lan"
	case "tailscale", "tailnet":
		return "tailscale"
	default:
		return bind
	}
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
				fmt.Println("\n🚀 Starting AssistClaw in background mode...")
				return Detach("start")
			}
			return nil
		},
	}
}

// collectProvider guides the user through selecting and configuring an LLM provider.
// openAICompatProviders is the list of provider keys that work with Plano
// (all OpenAI-compatible providers). Anthropic, Bedrock, and Vertex use their
// own proprietary protocols and are NOT OpenAI-compatible.
var openAICompatProviders = map[string]bool{
	"openai":     true,
	"azure":      true,
	"ollama":     true,
	"vllm":       true,
	"lmstudio":   true,
	"groq":       true,
	"mistral":    true,
	"openrouter": true,
	"deepseek":   true,
	"perplexity": true,
	"nvidia":     true,
	"xai":        true,
}

func collectProvider(theme *huh.Theme, providerType string, isPrimary bool, initial provEntry) (provEntry, error) {
	return collectProviderFiltered(theme, providerType, isPrimary, initial, false)
}

func collectProviderFiltered(theme *huh.Theme, providerType string, isPrimary bool, initial provEntry, planoMode bool) (provEntry, error) {
	entry := initial
	var providerOptions []huh.Option[string]

	allPrimary := []huh.Option[string]{
		huh.NewOption("Anthropic (Recommended)", "anthropic"),
		huh.NewOption("OpenAI", "openai"),
		huh.NewOption("Ollama (Local / Free)", "ollama"),
		huh.NewOption("AWS Bedrock", "bedrock"),
		huh.NewOption("Google Vertex AI", "vertex"),
		huh.NewOption("Groq", "groq"),
		huh.NewOption("Mistral", "mistral"),
		huh.NewOption("DeepSeek", "deepseek"),
		huh.NewOption("xAI (Grok)", "xai"),
		huh.NewOption("Perplexity", "perplexity"),
		huh.NewOption("OpenRouter", "openrouter"),
		huh.NewOption("NVIDIA NIM", "nvidia"),
		huh.NewOption("Together AI", "together"),
		huh.NewOption("HuggingFace (Inference API)", "huggingface"),
		huh.NewOption("Cohere", "cohere"),
		huh.NewOption("Azure OpenAI", "azure"),
		huh.NewOption("vLLM (Local / Custom)", "vllm"),
		huh.NewOption("LM Studio (Local)", "lmstudio"),
	}
	allSecondary := []huh.Option[string]{
		huh.NewOption("Ollama (Local / Free)", "ollama"),
		huh.NewOption("OpenAI", "openai"),
		huh.NewOption("Groq (Super Fast)", "groq"),
		huh.NewOption("Anthropic", "anthropic"),
		huh.NewOption("Mistral", "mistral"),
		huh.NewOption("OpenRouter", "openrouter"),
		huh.NewOption("Azure OpenAI", "azure"),
		huh.NewOption("DeepSeek", "deepseek"),
		huh.NewOption("xAI (Grok)", "xai"),
		huh.NewOption("Perplexity", "perplexity"),
		huh.NewOption("AWS Bedrock", "bedrock"),
		huh.NewOption("Google Vertex AI", "vertex"),
		huh.NewOption("NVIDIA NIM", "nvidia"),
		huh.NewOption("Together AI", "together"),
		huh.NewOption("HuggingFace (Inference API)", "huggingface"),
		huh.NewOption("Cohere", "cohere"),
		huh.NewOption("vLLM (Local / Custom)", "vllm"),
		huh.NewOption("LM Studio (Local)", "lmstudio"),
	}

	source := allSecondary
	if isPrimary {
		source = allPrimary
	}

	if planoMode {
		// Plano only routes to OpenAI-compatible providers
		for _, o := range source {
			if openAICompatProviders[o.Value] {
				providerOptions = append(providerOptions, o)
			}
		}
	} else {
		providerOptions = source
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

	// Prevent old config artifacts from leaking if user changed provider
	if entry.provider != initial.provider {
		entry.apiKey = ""
		entry.baseURL = ""
		entry.apiVersion = ""
		entry.awsRegion = ""
		entry.awsProfile = ""
		entry.awsAccessKey = ""
		entry.awsSecretKey = ""
		entry.vertexProj = ""
		entry.vertexLoc = ""
		entry.vertexCreds = ""
		entry.model = ""
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
		"xai":        true,
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
		if strings.TrimSpace(entry.baseURL) == "" {
			entry.baseURL = defaultURL
		}
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
		bedrockAuthMode := inferBedrockAuthMode(entry)
		if strings.TrimSpace(entry.awsRegion) == "" {
			entry.awsRegion = "us-east-1"
		}
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
		"xai": {
			huh.NewOption("Grok 4", "grok-4-latest"),
			huh.NewOption("Grok Beta", "grok-beta"),
			huh.NewOption("Grok Vision", "grok-vision-beta"),
			huh.NewOption("Grok 2", "grok-2"),
			huh.NewOption("Other / Custom...", "custom"),
		},
		"vertex": {
			huh.NewOption("Gemini 1.5 Pro", "gemini-1.5-pro"),
			huh.NewOption("Gemini 1.5 Flash", "gemini-1.5-flash"),
			huh.NewOption("Gemini 2.0 Flash Exp", "gemini-2.0-flash-exp"),
			huh.NewOption("Other / Custom...", "custom"),
		},
	}

	var modelOpts []huh.Option[string]
	if entry.provider == "bedrock" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		picks := bedrock.ListOnboardingTextModels(ctx, bedrock.Config{
			Region:          strings.TrimSpace(entry.awsRegion),
			Profile:         strings.TrimSpace(entry.awsProfile),
			AccessKeyID:     strings.TrimSpace(entry.awsAccessKey),
			SecretAccessKey: strings.TrimSpace(entry.awsSecretKey),
			APIKey:          strings.TrimSpace(entry.apiKey),
		})
		for _, p := range picks {
			modelOpts = append(modelOpts, huh.NewOption(p.Label, p.ID))
		}
		modelOpts = append(modelOpts, huh.NewOption("Other / Custom...", "custom"))
		modelOpts = ensureSelectValue(modelOpts, entry.model, "Current — ")
	} else if opts, ok := modelChoices[entry.provider]; ok {
		modelOpts = append([]huh.Option[string](nil), opts...)
		modelOpts = ensureSelectValue(modelOpts, entry.model, "Current — ")
	}

	if len(modelOpts) > 0 {
		desc := ""
		if entry.provider == "bedrock" {
			desc = "Lists ON_DEMAND text models in your region (AWS API) plus AssistClaw defaults."
		}
		formModel := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Model").
					Description(desc).
					Options(modelOpts...).
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
		tgDMMode         string = "pairing"
		tgAllowFromRaw   string
		dcBotToken       string
		dcDMMode         string = "pairing"
		dcAllowFromRaw   string
		slBotToken       string
		slAppToken       string
		slDMMode         string = "pairing"
		slAllowFromRaw   string
		waSessionID      string
		waDMMode         string = "pairing"
		waAllowFromRaw   string
		selectedSkills   []string
		codingModel        string
		visionModel        string
		tpl                string
		usePlano           bool
		planoEndpoint      string
		planoFastModel     string
		planoPowerfulModel string
	)

	theme := huh.ThemeBase()
	theme.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	theme.Focused.SelectedOption = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	theme.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	tui.PrintOnboardBanner(version)
	fmt.Println(lipgloss.NewStyle().Faint(true).Render("  Let's configure your autonomous agent environment."))
	fmt.Println()

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
			case "xai":
				if existing.Providers.XAI != nil {
					primary.apiKey = existing.Providers.XAI.APIKey
					primary.model = existing.Providers.XAI.DefaultModel
				}
			case "together":
				if existing.Providers.Together != nil {
					primary.apiKey = existing.Providers.Together.APIKey
					primary.baseURL = existing.Providers.Together.BaseURL
					primary.model = existing.Providers.Together.DefaultModel
				}
			case "nvidia":
				if existing.Providers.NVIDIA != nil {
					primary.apiKey = existing.Providers.NVIDIA.APIKey
					primary.baseURL = existing.Providers.NVIDIA.BaseURL
					primary.model = existing.Providers.NVIDIA.DefaultModel
				}
			case "cohere":
				if existing.Providers.Cohere != nil {
					primary.apiKey = existing.Providers.Cohere.APIKey
					primary.baseURL = existing.Providers.Cohere.BaseURL
					primary.model = existing.Providers.Cohere.DefaultModel
				}
			case "huggingface":
				if existing.Providers.HuggingFace != nil {
					primary.apiKey = existing.Providers.HuggingFace.APIKey
					primary.baseURL = existing.Providers.HuggingFace.BaseURL
					primary.model = existing.Providers.HuggingFace.DefaultModel
					if primary.model == "" && existing.Providers.HuggingFace.Model != "" {
						primary.model = existing.Providers.HuggingFace.Model
					}
				}
			case "voyage":
				if existing.Providers.Voyage != nil {
					primary.apiKey = existing.Providers.Voyage.APIKey
					primary.baseURL = existing.Providers.Voyage.BaseURL
					primary.model = existing.Providers.Voyage.DefaultModel
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
				case "xai":
					if existing.Providers.XAI != nil {
						secondary.apiKey = existing.Providers.XAI.APIKey
						secondary.model = existing.Providers.XAI.DefaultModel
					}
				case "together":
					if existing.Providers.Together != nil {
						secondary.apiKey = existing.Providers.Together.APIKey
						secondary.baseURL = existing.Providers.Together.BaseURL
						secondary.model = existing.Providers.Together.DefaultModel
					}
				case "nvidia":
					if existing.Providers.NVIDIA != nil {
						secondary.apiKey = existing.Providers.NVIDIA.APIKey
						secondary.baseURL = existing.Providers.NVIDIA.BaseURL
						secondary.model = existing.Providers.NVIDIA.DefaultModel
					}
				case "cohere":
					if existing.Providers.Cohere != nil {
						secondary.apiKey = existing.Providers.Cohere.APIKey
						secondary.baseURL = existing.Providers.Cohere.BaseURL
						secondary.model = existing.Providers.Cohere.DefaultModel
					}
				case "huggingface":
					if existing.Providers.HuggingFace != nil {
						secondary.apiKey = existing.Providers.HuggingFace.APIKey
						secondary.baseURL = existing.Providers.HuggingFace.BaseURL
						secondary.model = existing.Providers.HuggingFace.DefaultModel
						if secondary.model == "" && existing.Providers.HuggingFace.Model != "" {
							secondary.model = existing.Providers.HuggingFace.Model
						}
					}
				case "voyage":
					if existing.Providers.Voyage != nil {
						secondary.apiKey = existing.Providers.Voyage.APIKey
						secondary.baseURL = existing.Providers.Voyage.BaseURL
						secondary.model = existing.Providers.Voyage.DefaultModel
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
		gwMode = normalizeGatewayBind(existing.Gateway.Bind)
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

		if existing.Plano.Enabled {
			usePlano = true
			if strings.TrimSpace(existing.Plano.Endpoint) != "" {
				planoEndpoint = existing.Plano.Endpoint
			} else {
				planoEndpoint = "http://localhost:12000/v1"
			}
			prefs := existing.Plano.Preferences
			if len(prefs) >= 2 {
				planoFastModel = prefs[0].PreferModel
				planoPowerfulModel = prefs[1].PreferModel
			} else if len(prefs) == 1 {
				planoFastModel = prefs[0].PreferModel
			}
		}

		// Pre-populate Channels
		if existing.Channels.Telegram != nil {
			selectedChannels = append(selectedChannels, "telegram")
			tgBotToken = existing.Channels.Telegram.BotToken
			tgDMMode = existing.Channels.Telegram.DMMode
			if tgDMMode == "" {
				tgDMMode = "pairing"
			}
			tgAllowFromRaw = strings.Join(existing.Channels.Telegram.AllowFrom, ", ")
		}
		if existing.Channels.WhatsApp != nil {
			selectedChannels = append(selectedChannels, "whatsapp")
			waSessionID = existing.Channels.WhatsApp.SessionID
			waDMMode = existing.Channels.WhatsApp.DMMode
			if waDMMode == "" {
				waDMMode = "pairing"
			}
			waAllowFromRaw = strings.Join(existing.Channels.WhatsApp.AllowFrom, ", ")
		}
		if existing.Channels.Discord != nil {
			selectedChannels = append(selectedChannels, "discord")
			dcBotToken = existing.Channels.Discord.BotToken
			dcDMMode = existing.Channels.Discord.DMMode
			if dcDMMode == "" {
				dcDMMode = "pairing"
			}
			dcAllowFromRaw = strings.Join(existing.Channels.Discord.AllowFrom, ", ")
		}
		if existing.Channels.Slack != nil {
			selectedChannels = append(selectedChannels, "slack")
			slBotToken = existing.Channels.Slack.BotToken
			slAppToken = existing.Channels.Slack.AppToken
			slDMMode = existing.Channels.Slack.DMMode
			if slDMMode == "" {
				slDMMode = "pairing"
			}
			slAllowFromRaw = strings.Join(existing.Channels.Slack.AllowFrom, ", ")
		}

		// Pre-populate Skills
		selectedSkills = existing.Agent.EnabledSkills
	}

	// ────────────────────────────────────────────────────────────────────────
	// Step 1: Primary LLM Provider
	// ────────────────────────────────────────────────────────────────────────
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
		// Secondary provider — show all providers (no Plano filter yet)
		secondary, err = collectProvider(theme, "secondary", false, secondary)
		if err != nil {
			return false, err
		}
	} else {
		secondary.provider = "none"
	}

	// ─── Step 3: Plano Smart Routing ────────────────────────────────────────
	// Only offered when a secondary provider is configured.
	// Plano needs at least 2 OpenAI-compatible endpoints to route between.
	if secChoice == "configure" && secondary.provider != "" && secondary.provider != "none" {
		planoHint := ""
		if !openAICompatProviders[primary.provider] {
			planoHint = "\n\n⚠ Note: your primary provider (" + primary.provider + ") is not OpenAI-compatible.\n" +
				"Plano will route to your secondary provider for complex tasks."
		}
		_ = huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Smart Routing with Plano? (Optional — press Enter to skip)").
				Description(
					"Plano auto-routes each prompt by complexity: simple → fast/cheap model,\n" +
						"complex → powerful model. Cuts LLM costs 30–60%.\n" +
						"Requires Docker. Runs locally on port 12000." + planoHint,
				).
				Value(&usePlano),
		)).WithTheme(theme).Run()
	} // end if secChoice == "configure" && secondary

	if secChoice == "configure" && usePlano {
		if strings.TrimSpace(planoEndpoint) == "" {
			planoEndpoint = "http://localhost:12000/v1"
		}
		fastOpts := []huh.Option[string]{
			huh.NewOption("GPT-4o mini", "openai/gpt-4o-mini"),
			huh.NewOption("Groq Llama3 8B", "groq/llama3-8b-8192"),
			huh.NewOption("Mistral 7B", "mistral/mistral-7b-instruct"),
			huh.NewOption("Ollama Llama3.2", "ollama/llama3.2"),
			huh.NewOption("DeepSeek V2 (Lite)", "deepseek/deepseek-chat"),
		}
		fastOpts = ensureSelectValue(fastOpts, planoFastModel, "Current — ")
		powerOpts := []huh.Option[string]{
			huh.NewOption("GPT-4o", "openai/gpt-4o"),
			huh.NewOption("GPT-4.1", "openai/gpt-4.1"),
			huh.NewOption("Groq Llama3 70B", "groq/llama3-70b-8192"),
			huh.NewOption("Mistral Large", "mistral/mistral-large-latest"),
			huh.NewOption("Ollama Llama3.1 70B", "ollama/llama3.1:70b"),
			huh.NewOption("DeepSeek R1", "deepseek/deepseek-r1"),
		}
		powerOpts = ensureSelectValue(powerOpts, planoPowerfulModel, "Current — ")
		_ = huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Plano endpoint").
				Description("Leave as default if running Plano locally via Docker.").
				Value(&planoEndpoint),
			huh.NewSelect[string]().
				Title("Fast (cheap) model — for simple queries").
				Description("Used for greetings, short Q&A, and conversational turns.").
				Options(fastOpts...).
				Value(&planoFastModel),
			huh.NewSelect[string]().
				Title("Powerful model — for complex tasks").
				Description("Used for coding, multi-step reasoning, and analysis.").
				Options(powerOpts...).
				Value(&planoPowerfulModel),
		)).WithTheme(theme).Run()

		// ── Docker prerequisites ──────────────────────────────────────────────
		fmt.Println()
		dockerOK := setupPlanoDocker(planoEndpoint)
		if !dockerOK {
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(
				"  ⚠  Plano Docker setup skipped. Start it manually:\n" +
					"     docker run -d -p 12000:12000 --name plano katanemo/plano:latest",
			))
		}
		fmt.Println()
	}
	// ─────────────────────────────────────────────────────────────────────────

	codingOpts := []huh.Option[string]{
		huh.NewOption("Use Default", "default"),
		huh.NewOption("Claude 3.5 Sonnet", "anthropic/claude-3-5-sonnet-20241022"),
		huh.NewOption("GPT-4o", "openai/gpt-4o"),
		huh.NewOption("DeepSeek-R1 (Local)", "ollama/deepseek-r1"),
	}
	if codingModel == "" {
		codingModel = "default"
	}
	if visionModel == "" {
		visionModel = "default"
	}
	codingOpts = ensureSelectValue(codingOpts, codingModel, "Current — ")
	visionOpts := []huh.Option[string]{
		huh.NewOption("Use Default", "default"),
		huh.NewOption("Claude 3.5 Sonnet", "anthropic/claude-3-5-sonnet-20241022"),
		huh.NewOption("GPT-4o", "openai/gpt-4o"),
	}
	visionOpts = ensureSelectValue(visionOpts, visionModel, "Current — ")
	formRouting := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Advanced Routing: Coding").
				Options(codingOpts...).
				Value(&codingModel),
			huh.NewSelect[string]().
				Title("Advanced Routing: Vision").
				Options(visionOpts...).
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
		if strings.TrimSpace(embed.baseURL) == "" {
			embed.baseURL = "http://localhost:11434"
		}
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
		"cohere":  {huh.NewOption("embed-v4.0", "embed-v4.0")},
		"google":  {huh.NewOption("text-embedding-004", "text-embedding-004")},
		"voyage":  {huh.NewOption("voyage-3", "voyage-3"), huh.NewOption("voyage-3-lite", "voyage-3-lite")},
		"mistral": {huh.NewOption("mistral-embed", "mistral-embed")},
		"vertex":  {huh.NewOption("text-embedding-004", "text-embedding-004"), huh.NewOption("text-multilingual-embedding-002", "text-multilingual-embedding-002")},
	}
	var embedModelOpts []huh.Option[string]
	if embed.provider == "bedrock" {
		if primary.provider == "bedrock" {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			for _, p := range bedrock.ListOnboardingEmbeddingModels(ctx, bedrock.Config{
				Region:          strings.TrimSpace(primary.awsRegion),
				Profile:         strings.TrimSpace(primary.awsProfile),
				AccessKeyID:     strings.TrimSpace(primary.awsAccessKey),
				SecretAccessKey: strings.TrimSpace(primary.awsSecretKey),
				APIKey:          strings.TrimSpace(primary.apiKey),
			}) {
				embedModelOpts = append(embedModelOpts, huh.NewOption(p.Label, p.ID))
			}
		}
		if len(embedModelOpts) == 0 {
			embedModelOpts = []huh.Option[string]{
				huh.NewOption("Titan Text Embed v2", "amazon.titan-embed-text-v2:0"),
				huh.NewOption("Titan Text Embed v1", "amazon.titan-embed-text-v1"),
				huh.NewOption("Cohere English v3", "cohere.embed-english-v3"),
			}
		}
	} else if opts, ok := embedModels[embed.provider]; ok {
		embedModelOpts = append([]huh.Option[string](nil), opts...)
	}
	embedModelOpts = ensureSelectValue(embedModelOpts, embed.model, "Current — ")
	embedDesc := ""
	if embed.provider == "bedrock" {
		embedDesc = "Bedrock: when your primary LLM is also Bedrock, models are listed from AWS for your region."
	}
	embedFields = append(embedFields, huh.NewSelect[string]().Title("Embedding Model").Description(embedDesc).Options(embedModelOpts...).Value(&embed.model))

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

	if gwToken == "" {
		gwToken = randomToken(24)
		fmt.Printf("\n🔑 No security token provided. Generated a secure one for you:\n   %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render(gwToken))
	}

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

	tgLabel := "Telegram"
	dcLabel := "Discord"
	slLabel := "Slack"
	waLabel := "WhatsApp"

	if existing != nil {
		if existing.Channels.Telegram != nil && existing.Channels.Telegram.BotToken != "" {
			tgLabel = "Telegram [Configured]"
		}
		if existing.Channels.Discord != nil && existing.Channels.Discord.BotToken != "" {
			dcLabel = "Discord [Configured]"
		}
		if existing.Channels.Slack != nil && existing.Channels.Slack.BotToken != "" {
			slLabel = "Slack [Configured]"
		}
		if existing.Channels.WhatsApp != nil {
			waLabel = "WhatsApp [Configured]"
		}
	}

	formChannels := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Messaging Channels").
				Options(
					huh.NewOption(tgLabel, "telegram"),
					huh.NewOption(dcLabel, "discord"),
					huh.NewOption(slLabel, "slack"),
					huh.NewOption(waLabel, "whatsapp"),
				).
				Value(&selectedChannels),
		),
	).WithTheme(theme)
	_ = formChannels.Run()

	for _, ch := range selectedChannels {
		switch ch {
		case "telegram":
			_ = huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("Telegram Bot Token").
					Value(&tgBotToken),
				huh.NewSelect[string]().
					Title("Telegram Security Mode").
					Description("pairing: Approve new chatters. allowlist: Only whitelisted. open: Public.").
					Options(
						huh.NewOption("Pairing (Recommended)", "pairing"),
						huh.NewOption("Allowlist only", "allowlist"),
						huh.NewOption("Open (Public)", "open"),
						huh.NewOption("Disabled", "disabled"),
					).
					Value(&tgDMMode),
				huh.NewInput().
					Title("Whitelisted Telegram IDs").
					Description("Comma-separated (numeric IDs or @usernames). Only for Allowlist mode.").
					Value(&tgAllowFromRaw),
			)).WithTheme(theme).Run()
		case "discord":
			_ = huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("Discord Bot Token").
					Value(&dcBotToken),
				huh.NewSelect[string]().
					Title("Discord Security Mode").
					Description("pairing: Approve new chatters. allowlist: Only whitelisted. open: Public.").
					Options(
						huh.NewOption("Pairing (Recommended)", "pairing"),
						huh.NewOption("Allowlist only", "allowlist"),
						huh.NewOption("Open (Public)", "open"),
						huh.NewOption("Disabled", "disabled"),
					).
					Value(&dcDMMode),
				huh.NewInput().
					Title("Whitelisted Discord IDs").
					Description("Comma-separated numeric IDs. Only for Allowlist mode.").
					Value(&dcAllowFromRaw),
			)).WithTheme(theme).Run()
		case "slack":
			_ = huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("Slack Bot Token").
					Value(&slBotToken),
				huh.NewInput().
					Title("Slack App Token").
					Value(&slAppToken),
				huh.NewSelect[string]().
					Title("Slack Security Mode").
					Description("pairing: Approve new chatters. allowlist: Only whitelisted. open: Public.").
					Options(
						huh.NewOption("Pairing (Recommended)", "pairing"),
						huh.NewOption("Allowlist only", "allowlist"),
						huh.NewOption("Open (Public)", "open"),
						huh.NewOption("Disabled", "disabled"),
					).
					Value(&slDMMode),
				huh.NewInput().
					Title("Whitelisted Slack IDs").
					Description("Comma-separated numeric IDs. Only for Allowlist mode.").
					Value(&slAllowFromRaw),
			)).WithTheme(theme).Run()
		case "whatsapp":
			if strings.TrimSpace(waSessionID) == "" {
				waSessionID = "personal"
			}
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("\n--- WhatsApp Integration ---"))
			fmt.Println("AssistClaw acts as a standalone WhatsApp account.")
			fmt.Println("You will need to scan a QR code using 'Linked Devices' on your phone.")

			_ = huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title("WhatsApp Session ID").
					Description("A name for this session (e.g. 'personal')").
					Value(&waSessionID),
				huh.NewSelect[string]().
					Title("WhatsApp Security Mode").
					Description("pairing: Approve new chatters. allowlist: Only whitelisted. open: Public.").
					Options(
						huh.NewOption("Pairing (Recommended)", "pairing"),
						huh.NewOption("Allowlist only", "allowlist"),
						huh.NewOption("Open (Public)", "open"),
						huh.NewOption("Disabled", "disabled"),
					).
					Value(&waDMMode),
				huh.NewInput().
					Title("Whitelisted Numbers").
					Description("Comma-separated (e.g. '1234567890, 9876543210'). Only for Allowlist mode.").
					Value(&waAllowFromRaw),
			)).WithTheme(theme).Run()

			var linkNow bool
			_ = huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title("Link WhatsApp now (QR code)?").
					Description("Recommended: Scan the QR code now to complete setup.").
					Value(&linkNow),
			)).WithTheme(theme).Run()

			if linkNow {
				parts := strings.Split(waAllowFromRaw, ",")
				var allowFrom []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						allowFrom = append(allowFrom, p)
					}
				}
				dbPath := filepath.Join(config.DefaultConfigPath(), "..", "whatsapp.db")
				if home, err := os.UserHomeDir(); err == nil {
					dbPath = filepath.Join(home, ".assistclaw", "whatsapp.db")
				}
				wa, err := whatsapp.New(dbPath, waSessionID, waDMMode, allowFrom, "INFO", nil)
				if err == nil {
					if !wa.IsLinked() {
						fmt.Println("\n--- WhatsApp Pairing ---")
						_ = wa.Connect(context.Background())
						_ = wa.Stop() // Terminate onboarding connection to avoid conflict with agent
					}
					fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ WhatsApp Linked! Continuing setup..."))
				} else {
					fmt.Printf("Error initializing WhatsApp for pairing: %v\n", err)
				}
			}
		}
	}

	// Skill configuration — identical TUI used by "assistclaw skills configure"
	{
		home, _ := os.UserHomeDir()
		bundledDir := filepath.Join(home, ".assistclaw", "skills", "bundled")
		customDir := filepath.Join(home, ".assistclaw", "skills", "custom")
		_ = os.MkdirAll(bundledDir, 0o755)
		_ = os.MkdirAll(customDir, 0o755)

		// Extract bundled skills from repo if available
		if src := resolveBundledSkillsSrc(); src != "" {
			_ = skills.CopyDir(src, bundledDir)
		}

		mp := skills.NewMarketplace(bundledDir, customDir)
		idx, err := mp.FetchIndex(context.Background())
		if err == nil {
			type skillOption struct {
				name string
			}

			const customSentinel = "__custom__"
			var opts []huh.Option[string]
			inIndex := make(map[string]bool)
			for _, e := range idx.Skills {
				inIndex[e.Name] = true
				label := e.Name
				if e.Emoji != "" {
					label = e.Emoji + "  " + e.Name
				}
				if e.Description != "" {
					desc := e.Description
					if len(desc) > 72 {
						desc = desc[:71] + "…"
					}
					label += "\n     " + desc
				}
				opts = append(opts, huh.NewOption(label, e.Name))
			}
			for _, name := range selectedSkills {
				if name != "" && !inIndex[name] {
					opts = append(opts, huh.NewOption(name+" (from config)", name))
				}
			}
			opts = append(opts, huh.NewOption("＋  Add custom skill  (local path or URL)", customSentinel))

			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).
				Render("\n  🛠  Select Agent Skills"))
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
				Render("  Space to toggle · Enter to confirm · ↑↓ to move\n"))

			skillPick := append([]string(nil), selectedSkills...)
			_ = huh.NewForm(huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Enable Skills").
					Description("Bundled skills are fetched automatically if not present locally.").
					Options(opts...).
					Value(&skillPick),
			)).WithTheme(theme).Run()

			selectedSkills = nil
			var customPath string
			for _, n := range skillPick {
				if n == customSentinel {
					_ = huh.NewForm(huh.NewGroup(
						huh.NewInput().
							Title("Custom skill path or URL").
							Placeholder("/path/to/skill  or  https://github.com/user/skill").
							Value(&customPath),
					)).WithTheme(theme).Run()
					if customPath != "" {
						dest, err := mp.InstallFromPath(customPath)
						if err != nil {
							// Try as URL
							dest, err = mp.Install(context.Background(), customPath)
						}
						if err == nil {
							selectedSkills = append(selectedSkills, filepath.Base(dest))
							fmt.Printf("✔ Custom skill installed: %s\n", filepath.Base(dest))
						} else {
							fmt.Printf("⚠ Custom skill failed: %v\n", err)
						}
					}
				} else {
					selectedSkills = append(selectedSkills, n)
					// Install in background (non-blocking feedback)
					dest := filepath.Join(customDir, n)
					if _, err := os.Stat(dest); os.IsNotExist(err) {
						fmt.Printf("  Installing %s...", n)
						if _, err := mp.Install(context.Background(), n); err != nil {
							fmt.Printf(" ⚠ %v\n", err)
						} else {
							fmt.Println(" ✔")
						}
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
		sb.WriteString(fmt.Sprintf("    default_model: \"%s\"\n", e.model))
	}
	writeEmbed(embed)
	sb.WriteString("\n")

	if len(selectedChannels) > 0 {
		sb.WriteString("channels:\n")
		for _, ch := range selectedChannels {
			sb.WriteString(fmt.Sprintf("  %s:\n", ch))
			if ch == "telegram" {
				sb.WriteString(fmt.Sprintf("    bot_token: \"%s\"\n", tgBotToken))
				sb.WriteString(fmt.Sprintf("    dm_mode: \"%s\"\n", tgDMMode))
				if tgAllowFromRaw != "" {
					sb.WriteString("    allow_from:\n")
					parts := strings.Split(tgAllowFromRaw, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							sb.WriteString(fmt.Sprintf("      - \"%s\"\n", p))
						}
					}
				}
			}
			if ch == "discord" {
				sb.WriteString(fmt.Sprintf("    bot_token: \"%s\"\n", dcBotToken))
				sb.WriteString(fmt.Sprintf("    dm_mode: \"%s\"\n", dcDMMode))
				if dcAllowFromRaw != "" {
					sb.WriteString("    allow_from:\n")
					parts := strings.Split(dcAllowFromRaw, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							sb.WriteString(fmt.Sprintf("      - \"%s\"\n", p))
						}
					}
				}
			}
			if ch == "slack" {
				sb.WriteString(fmt.Sprintf("    bot_token: \"%s\"\n", slBotToken))
				sb.WriteString(fmt.Sprintf("    app_token: \"%s\"\n", slAppToken))
				sb.WriteString(fmt.Sprintf("    dm_mode: \"%s\"\n", slDMMode))
				if slAllowFromRaw != "" {
					sb.WriteString("    allow_from:\n")
					parts := strings.Split(slAllowFromRaw, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							sb.WriteString(fmt.Sprintf("      - \"%s\"\n", p))
						}
					}
				}
			}
			if ch == "whatsapp" {
				sb.WriteString(fmt.Sprintf("    session_id: \"%s\"\n", waSessionID))
				sb.WriteString(fmt.Sprintf("    dm_mode: \"%s\"\n", waDMMode))
				if waAllowFromRaw != "" {
					sb.WriteString("    allow_from:\n")
					parts := strings.Split(waAllowFromRaw, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							sb.WriteString(fmt.Sprintf("      - \"%s\"\n", p))
						}
					}
				}
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

	// Plano smart routing config
	if usePlano {
		sb.WriteString("\nplano:\n")
		sb.WriteString("  enabled: true\n")
		sb.WriteString(fmt.Sprintf("  endpoint: \"%s\"\n", planoEndpoint))
		sb.WriteString("  preferences:\n")
		sb.WriteString("    - description: \"Route simple greetings, weather, casual chat, and short questions to the fast model\"\n")
		sb.WriteString(fmt.Sprintf("      prefer_model: \"%s\"\n", planoFastModel))
		sb.WriteString("    - description: \"Route code generation, debugging, analysis, and multi-step reasoning to the powerful model\"\n")
		sb.WriteString(fmt.Sprintf("      prefer_model: \"%s\"\n", planoPowerfulModel))
	}

	tpl = sb.String()
	_ = os.MkdirAll(filepath.Dir(configPath), 0o755)
	_ = os.WriteFile(configPath, []byte(tpl), 0o600)

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Configuration saved!"))

	// ── Auto-start on login ───────────────────────────────────────────────────
	var installSvc bool
	_ = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Start AssistClaw automatically on login?").
			Description("Installs a system service (launchd on macOS, systemd on Linux).\nThe web UI will be available at http://localhost:" + fmt.Sprintf("%d", gwPort) + " after every reboot.").
			Value(&installSvc),
	)).WithTheme(theme).Run()

	if installSvc {
		if err := installService(); err != nil {
			fmt.Printf("\n⚠  Service install failed: %v\n", err)
			fmt.Println("   You can retry later with: assistclaw service install")
		}
	}

	// ── Launch summary banner ─────────────────────────────────────────────────
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	webURL := fmt.Sprintf("http://localhost:%d", gwPort)

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("  🚀 Launching AssistClaw in background…"))
	fmt.Println()
	fmt.Printf("  %-14s %s\n", dim.Render("Web UI:"), accent.Render(webURL))
	fmt.Printf("  %-14s %s\n", dim.Render("Token:"), accent.Render(gwToken))
	fmt.Printf("  %-14s %s\n", dim.Render("Gateway:"), accent.Render(fmt.Sprintf("%s:%d", gwHost, gwPort)))
	fmt.Println()
	fmt.Println(dim.Render("  Manage with:"))
	fmt.Println(dim.Render("    assistclaw gateway start   │ stop   │ restart"))
	fmt.Println(dim.Render("    assistclaw service install          (auto-login)"))
	fmt.Println(dim.Render("    assistclaw status"))
	if len(selectedChannels) > 0 {
		fmt.Println()
		fmt.Println(dim.Render("  Tip: Scan the QR code in the terminal to link WhatsApp."))
	}
	fmt.Println()

	// Always start daemon after onboarding completes
	return true, nil
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// setupPlanoDocker checks Docker availability, pulls the Plano image, and
// starts a detached Plano container listening on port 12000.
// Returns true if Plano is confirmed reachable after setup.
func setupPlanoDocker(endpoint string) bool {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	// 1. Check docker binary
	fmt.Print("  Checking Docker... ")
	if err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		fmt.Println(red.Render("✗ Docker not found."))
		fmt.Println(yellow.Render("  Install Docker Desktop: https://docs.docker.com/get-docker/"))
		return false
	}
	fmt.Println(green.Render("✔ Docker found"))

	// 2. Check if plano container already running
	out, _ := exec.Command("docker", "ps", "--filter", "name=plano", "--format", "{{.Names}}").Output()
	if strings.Contains(string(out), "plano") {
		fmt.Println(green.Render("  ✔ Plano container already running"))
		return waitForPlano(endpoint, 5)
	}

	// 3. Pull image
	fmt.Print("  Pulling katanemo/plano:latest (this may take a minute)... ")
	pullCmd := exec.Command("docker", "pull", "katanemo/plano:latest")
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Println(red.Render("✗ Pull failed: " + err.Error()))
		return false
	}
	fmt.Println(green.Render("  ✔ Image pulled"))

	// 4. Remove any stopped plano container
	_ = exec.Command("docker", "rm", "-f", "plano").Run()

	// 5. Start container detached
	fmt.Print("  Starting Plano container... ")
	startCmd := exec.Command("docker", "run", "-d",
		"--name", "plano",
		"-p", "12000:12000",
		"--restart", "unless-stopped",
		"katanemo/plano:latest",
	)
	if out, err := startCmd.CombinedOutput(); err != nil {
		fmt.Println(red.Render("✗ Failed to start: " + string(out)))
		return false
	}
	fmt.Println(green.Render("✔ Container started"))

	// 6. Wait for readiness
	fmt.Print("  Waiting for Plano to be ready")
	if waitForPlano(endpoint, 15) {
		fmt.Println(" " + green.Render("✔ Ready!"))
		return true
	}
	fmt.Println(" " + yellow.Render("⚠ Timed out — Plano may still be starting up"))
	return false
}

// waitForPlano polls the Plano /v1/models endpoint until it responds or timeout expires.
func waitForPlano(endpoint string, timeoutSec int) bool {
	modelsURL := strings.TrimRight(endpoint, "/") + "/models"
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := client.Get(modelsURL); err == nil && resp.StatusCode < 400 {
			resp.Body.Close()
			return true
		}
		fmt.Print(".")
		time.Sleep(1 * time.Second)
	}
	return false
}
