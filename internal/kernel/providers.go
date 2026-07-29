// Package kernel is AssistClaw's composition root: the one place allowed to
// import every subsystem and wire them into a running application. It exists to
// pull construction/wiring logic out of the cmd/assistclaw entry point (which
// had grown a ~960-line runAgent god function) and make it independently
// testable. Everything kernel builds is expressed in terms of internal/core
// contracts where possible.
//
// This file holds provider and embedder registration, moved verbatim from
// cmd/assistclaw/main.go. cmd retains thin wrappers that delegate here so the
// many existing call sites compile unchanged.
package kernel

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	embedproviders "github.com/assistclaw/assistclaw/internal/embeddings/providers"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/provider/anthropic"
	"github.com/assistclaw/assistclaw/internal/provider/bedrock"
	"github.com/assistclaw/assistclaw/internal/provider/catalogs"
	"github.com/assistclaw/assistclaw/internal/provider/ollama"
	"github.com/assistclaw/assistclaw/internal/provider/openai"
	"github.com/assistclaw/assistclaw/internal/provider/openaicompat"
	planoprovider "github.com/assistclaw/assistclaw/internal/provider/plano"
	"github.com/assistclaw/assistclaw/internal/provider/vertex"
)

// BuildLogger constructs the process zap logger at the given level.
func BuildLogger(level string) *zap.Logger {
	lvl := zap.WarnLevel
	switch strings.ToLower(level) {
	case "debug":
		lvl = zap.DebugLevel
	case "info":
		lvl = zap.InfoLevel
	case "warn":
		lvl = zap.WarnLevel
	case "error":
		lvl = zap.ErrorLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.EncoderConfig.TimeKey = "t"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	log, _ := cfg.Build()
	return log
}

// RegisterProviders registers every configured LLM backend into reg.
func RegisterProviders(ctx context.Context, cfg *config.Config, reg *provider.Registry, log *zap.Logger) error {
	prov := cfg.Providers
	register := func(p provider.Provider) {
		if err := reg.Register(ctx, p); err != nil {
			log.Warn("provider registration warning", zap.String("provider", p.Name()), zap.Error(err))
		}
	}

	if prov.OpenAI != nil {
		register(openai.New(openai.Config{
			APIKey: prov.OpenAI.APIKey, BaseURL: prov.OpenAI.BaseURL,
			DefaultModel: prov.OpenAI.DefaultModel,
		}))
	}
	if prov.AzureOpenAI != nil {
		register(openai.New(openai.Config{
			APIKey: prov.AzureOpenAI.APIKey, BaseURL: prov.AzureOpenAI.BaseURL,
			IsAzure: true, APIVersion: prov.AzureOpenAI.APIVersion,
		}))
	}
	if prov.Anthropic != nil {
		register(anthropic.New(anthropic.Config{
			APIKey: prov.Anthropic.APIKey, BaseURL: prov.Anthropic.BaseURL,
			DefaultModel: prov.Anthropic.DefaultModel,
		}))
	}
	if prov.Bedrock != nil {
		p, err := bedrock.New(bedrock.Config{
			Region: prov.Bedrock.Region, Profile: prov.Bedrock.Profile,
			AccessKeyID: prov.Bedrock.AccessKeyID, SecretAccessKey: prov.Bedrock.SecretAccessKey,
			APIKey:       prov.Bedrock.APIKey,
			DefaultModel: prov.Bedrock.DefaultModel,
		})
		if err != nil {
			log.Warn("bedrock init failed", zap.Error(err))
		} else {
			register(p)
		}
	}
	if prov.Ollama != nil {
		register(ollama.New(ollama.Config{
			BaseURL: prov.Ollama.BaseURL, DefaultModel: prov.Ollama.DefaultModel,
		}))
	}
	if prov.VLLM != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "vllm", BaseURL: prov.VLLM.BaseURL, APIKey: prov.VLLM.APIKey,
			DefaultModel: prov.VLLM.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.LMStudio != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "lmstudio", BaseURL: prov.LMStudio.BaseURL,
			DefaultModel: prov.LMStudio.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.Groq != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "groq", BaseURL: "https://api.groq.com", APIKey: prov.Groq.APIKey,
			DefaultModel:   prov.Groq.DefaultModel,
			StaticModels:   catalogs.GroqModels("groq"),
			DiscoverModels: true,
		}))
	}
	if prov.Mistral != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "mistral", BaseURL: "https://api.mistral.ai", APIKey: prov.Mistral.APIKey,
			DefaultModel:   prov.Mistral.DefaultModel,
			StaticModels:   catalogs.MistralModels("mistral"),
			DiscoverModels: true,
		}))
	}
	if prov.Together != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "together", BaseURL: "https://api.together.xyz", APIKey: prov.Together.APIKey,
			DefaultModel:   prov.Together.DefaultModel,
			StaticModels:   catalogs.TogetherModels("together"),
			DiscoverModels: true,
		}))
	}
	if prov.OpenRouter != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "openrouter", BaseURL: "https://openrouter.ai/api", APIKey: prov.OpenRouter.APIKey,
			DefaultModel:   prov.OpenRouter.DefaultModel,
			StaticModels:   catalogs.OpenRouterModels("openrouter"),
			DiscoverModels: true,
			ExtraHeaders: map[string]string{
				"HTTP-Referer": prov.OpenRouter.SiteURL,
				"X-Title":      prov.OpenRouter.SiteName,
			},
		}))
	}
	if prov.NVIDIA != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "nvidia", BaseURL: "https://integrate.api.nvidia.com", APIKey: prov.NVIDIA.APIKey,
			DefaultModel:   prov.NVIDIA.DefaultModel,
			StaticModels:   catalogs.NVIDIAModels("nvidia"),
			DiscoverModels: true,
		}))
	}
	if prov.Cohere != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "cohere", BaseURL: "https://api.cohere.com", APIKey: prov.Cohere.APIKey,
			DefaultModel:   prov.Cohere.DefaultModel,
			StaticModels:   catalogs.CohereModels("cohere"),
			DiscoverModels: true,
		}))
	}
	if prov.HuggingFace != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "huggingface", BaseURL: prov.HuggingFace.BaseURL, APIKey: prov.HuggingFace.APIKey,
			DefaultModel: prov.HuggingFace.DefaultModel,
		}))
	}
	if prov.DeepSeek != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: prov.DeepSeek.APIKey,
			DefaultModel:   prov.DeepSeek.DefaultModel,
			StaticModels:   catalogs.DeepSeekModels("deepseek"),
			DiscoverModels: true,
		}))
	}
	if prov.Perplexity != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "perplexity", BaseURL: "https://api.perplexity.ai", APIKey: prov.Perplexity.APIKey,
			DefaultModel:   prov.Perplexity.DefaultModel,
			StaticModels:   catalogs.PerplexityModels("perplexity"),
			DiscoverModels: true,
		}))
	}
	if prov.XAI != nil {
		xaiModel := prov.XAI.DefaultModel
		if xaiModel == "" {
			xaiModel = "grok-4"
		}
		register(openaicompat.New(openaicompat.Config{
			Name:           "xai",
			BaseURL:        "https://api.x.ai/v1",
			APIKey:         prov.XAI.APIKey,
			DefaultModel:   xaiModel,
			StaticModels:   catalogs.XAIModels("xai"),
			DiscoverModels: true,
		}))
	}
	if prov.Vertex != nil {
		v, err := vertex.New(ctx, vertex.Config{
			ProjectID:    prov.Vertex.ProjectID,
			Location:     prov.Vertex.Location,
			Credentials:  prov.Vertex.Credentials,
			DefaultModel: prov.Vertex.DefaultModel,
		})
		if err != nil {
			log.Warn("vertex init failed", zap.Error(err))
		} else {
			register(v)
		}
	}
	// ─── Plano Smart Routing ───────────────────────────────────────────────────
	// If Plano is enabled, register it as the primary provider so all requests
	// flow through Plano's complexity-aware router. All other providers are still
	// registered so Plano can delegate to them, and as fallback if Plano is down.
	if cfg.Plano.Enabled {
		// Convert config.PlanoPreference → planoprovider.Preference
		prefs := make([]planoprovider.Preference, len(cfg.Plano.Preferences))
		for i, p := range cfg.Plano.Preferences {
			prefs[i] = planoprovider.Preference{
				Description: p.Description,
				PreferModel: p.PreferModel,
			}
		}

		// Look up fallback from already-registered providers
		var fallback provider.Provider
		if cfg.Plano.FallbackProvider != "" {
			if p, ok := reg.Get(cfg.Plano.FallbackProvider); ok {
				fallback = p
			}
		}

		planoP := planoprovider.New(planoprovider.Config{
			Enabled:          true,
			Endpoint:         cfg.Plano.Endpoint,
			FallbackProvider: cfg.Plano.FallbackProvider,
			Preferences:      prefs,
		}, fallback)

		register(planoP)
		log.Info("Plano smart routing enabled",
			zap.String("endpoint", cfg.Plano.Endpoint),
			zap.Int("preferences", len(prefs)),
		)
	}
	// ──────────────────────────────────────────────────────────────────────────

	return nil
}

// mergeBedrockForEmbeds fills in embeddings.bedrock from providers.bedrock when the embeddings
// block only sets default_model (or is partial). Otherwise LoadDefaultConfig falls through to
// EC2 IMDS and hangs or errors on laptops.
func mergeBedrockForEmbeds(ec *config.BedrockCreds, prov *config.BedrockCreds) *config.BedrockCreds {
	if ec == nil && prov == nil {
		return nil
	}
	var out config.BedrockCreds
	if ec != nil {
		out = *ec
	}
	if prov != nil {
		if out.Region == "" {
			out.Region = prov.Region
		}
		if out.Profile == "" {
			out.Profile = prov.Profile
		}
		if out.AccessKeyID == "" {
			out.AccessKeyID = prov.AccessKeyID
		}
		if out.SecretAccessKey == "" {
			out.SecretAccessKey = prov.SecretAccessKey
		}
		if out.APIKey == "" {
			out.APIKey = prov.APIKey
		}
		if out.DefaultModel == "" {
			out.DefaultModel = prov.DefaultModel
		}
	}
	if out.Region == "" {
		out.Region = "us-east-1"
	}
	return &out
}

// RegisterEmbedders registers every configured embedding backend into reg, in
// the configured priority order.
func RegisterEmbedders(ctx context.Context, cfg *config.Config, reg *embeddings.Registry, log *zap.Logger) {
	ec := cfg.Embeddings
	register := func(e embeddings.Embedder) {
		if err := reg.Register(ctx, e); err != nil {
			log.Warn("embedder registration warning", zap.String("provider", e.Name()), zap.Error(err))
		}
	}

	// Register in priority order
	for _, name := range ec.Priority {
		switch name {
		case "openai":
			if ec.OpenAI != nil {
				register(embedproviders.NewOpenAI(ec.OpenAI.APIKey, ec.OpenAI.BaseURL))
			} else if cfg.Providers.OpenAI != nil {
				register(embedproviders.NewOpenAI(cfg.Providers.OpenAI.APIKey, ""))
			}
		case "azure":
			if ec.AzureOpenAI != nil {
				register(embedproviders.NewAzure(ec.AzureOpenAI.APIKey, ec.AzureOpenAI.BaseURL, ec.AzureOpenAI.APIVersion))
			} else if cfg.Providers.AzureOpenAI != nil {
				register(embedproviders.NewAzure(cfg.Providers.AzureOpenAI.APIKey, cfg.Providers.AzureOpenAI.BaseURL, cfg.Providers.AzureOpenAI.APIVersion))
			}
		case "ollama":
			if ec.OllamaEmbed != nil {
				register(embedproviders.NewOllama(ec.OllamaEmbed.BaseURL))
			} else if cfg.Providers.Ollama != nil {
				register(embedproviders.NewOllama(cfg.Providers.Ollama.BaseURL))
			} else {
				register(embedproviders.NewOllama(""))
			}
		case "bedrock":
			b := mergeBedrockForEmbeds(ec.Bedrock, cfg.Providers.Bedrock)
			if b != nil {
				e, err := embedproviders.NewBedrock(b.Region, b.Profile, b.AccessKeyID, b.SecretAccessKey, b.APIKey)
				if err != nil {
					log.Warn("bedrock embedder failed to initialize; semantic memory may be unavailable",
						zap.Error(err))
				} else {
					register(e)
				}
			}
		case "cohere":
			if ec.Cohere != nil {
				register(embedproviders.NewCohere(ec.Cohere.APIKey))
			} else if cfg.Providers.Cohere != nil {
				register(embedproviders.NewCohere(cfg.Providers.Cohere.APIKey))
			}
		case "google":
			if ec.Google != nil {
				register(embedproviders.NewGoogle(ec.Google.APIKey))
			}
		case "huggingface":
			if ec.HuggingFace != nil {
				register(embedproviders.NewHuggingFace(ec.HuggingFace.APIKey, ec.HuggingFace.BaseURL, ec.HuggingFace.Model))
			}
		case "voyage":
			if ec.Voyage != nil {
				register(embedproviders.NewVoyage(ec.Voyage.APIKey, ec.Voyage.BaseURL))
			} else if cfg.Providers.Voyage != nil {
				register(embedproviders.NewVoyage(cfg.Providers.Voyage.APIKey, cfg.Providers.Voyage.BaseURL))
			}
		case "mistral":
			if ec.Mistral != nil {
				register(embedproviders.NewMistral(ec.Mistral.APIKey, ec.Mistral.BaseURL))
			} else if cfg.Providers.Mistral != nil {
				register(embedproviders.NewMistral(cfg.Providers.Mistral.APIKey, ""))
			}
		case "vertex":
			v := ec.Vertex
			if v == nil && cfg.Providers.Vertex != nil {
				v = cfg.Providers.Vertex
			}
			if v != nil {
				e, err := embedproviders.NewVertex(ctx, v.ProjectID, v.Location, v.Credentials)
				if err == nil {
					register(e)
				}
			}
		}
	}
}
