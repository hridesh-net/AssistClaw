// Package providers wires up all LLM provider instances using config.
// Each provider is a thin constructor on top of the openaicompat base or
// a custom implementation (Anthropic, Bedrock, Vertex, Ollama).
package providers

import (
	"sync"

	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/provider/anthropic"
	"github.com/assistclaw/assistclaw/internal/provider/ollama"
	"github.com/assistclaw/assistclaw/internal/provider/openai"
	"github.com/assistclaw/assistclaw/internal/provider/openaicompat"
)

// Config holds all provider configurations loaded from assistclaw.yaml.
type Config struct {
	OpenAI      *OpenAIConfig      `yaml:"openai"`
	AzureOpenAI *AzureOpenAIConfig `yaml:"azure_openai"`
	Anthropic   *AnthropicConfig   `yaml:"anthropic"`
	Ollama      *OllamaConfig      `yaml:"ollama"`
	VLLM        *VLLMConfig        `yaml:"vllm"`
	LMStudio    *LMStudioConfig    `yaml:"lm_studio"`
	Groq        *GroqConfig        `yaml:"groq"`
	Mistral     *MistralConfig     `yaml:"mistral"`
	Together    *TogetherConfig    `yaml:"together"`
	OpenRouter  *OpenRouterConfig  `yaml:"openrouter"`
	NVIDIA      *NVIDIAConfig      `yaml:"nvidia"`
	Cohere      *CohereConfig      `yaml:"cohere"`
	DeepSeek    *DeepSeekConfig    `yaml:"deepseek"`
	Perplexity  *PerplexityConfig  `yaml:"perplexity"`
	XAI         *XAIConfig         `yaml:"xai"`
	HuggingFace *HuggingFaceConfig `yaml:"huggingface"`
}

type OpenAIConfig struct {
	APIKey       string `yaml:"api_key"`
	OrgID        string `yaml:"org_id"`
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
}

type AzureOpenAIConfig struct {
	APIKey       string `yaml:"api_key"`
	BaseURL      string `yaml:"base_url"`
	APIVersion   string `yaml:"api_version"`
	DefaultModel string `yaml:"default_model"`
}

type AnthropicConfig struct {
	APIKey       string   `yaml:"api_key"`
	BaseURL      string   `yaml:"base_url"`
	DefaultModel string   `yaml:"default_model"`
	BetaFeatures []string `yaml:"beta_features"`
}

type OllamaConfig struct {
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
}

type VLLMConfig struct {
	BaseURL      string `yaml:"base_url"`
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type LMStudioConfig struct {
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
}

type GroqConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type MistralConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type TogetherConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type OpenRouterConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
	SiteName     string `yaml:"site_name"`
	SiteURL      string `yaml:"site_url"`
}

type NVIDIAConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type CohereConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type HuggingFaceConfig struct {
	APIKey       string `yaml:"api_key"`
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
}

type DeepSeekConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type PerplexityConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

type XAIConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
}

// ConfigBuilder materializes zero or more [provider.Provider] values from YAML config.
type ConfigBuilder func(*Config) []provider.Provider

// providerConfigBuilders is the ordered wiring list for first-party backends.
var providerConfigBuilders = []ConfigBuilder{
	appendOpenAI,
	appendAzureOpenAI,
	appendAnthropic,
	appendOllama,
	appendVLLM,
	appendLMStudio,
	appendGroq,
	appendMistral,
	appendTogether,
	appendOpenRouter,
	appendNVIDIA,
	appendCohere,
	appendHuggingFace,
	appendDeepSeek,
	appendPerplexity,
	appendXAI,
}

var (
	extraBuildersMu sync.Mutex
	extraBuilders   []ConfigBuilder
)

// RegisterConfigBuilder registers an extra builder, e.g. from init() in an
// out-of-tree module. Extra builders run after [providerConfigBuilders], in
// registration order.
func RegisterConfigBuilder(b ConfigBuilder) {
	if b == nil {
		return
	}
	extraBuildersMu.Lock()
	extraBuilders = append(extraBuilders, b)
	extraBuildersMu.Unlock()
}

func appendOpenAI(cfg *Config) []provider.Provider {
	if cfg.OpenAI == nil {
		return nil
	}
	return []provider.Provider{openai.New(openai.Config{
		APIKey:       cfg.OpenAI.APIKey,
		BaseURL:      cfg.OpenAI.BaseURL,
		OrgID:        cfg.OpenAI.OrgID,
		DefaultModel: orDefault(cfg.OpenAI.DefaultModel, "gpt-4o-mini"),
	})}
}

func appendAzureOpenAI(cfg *Config) []provider.Provider {
	if cfg.AzureOpenAI == nil {
		return nil
	}
	return []provider.Provider{openai.New(openai.Config{
		APIKey:       cfg.AzureOpenAI.APIKey,
		BaseURL:      cfg.AzureOpenAI.BaseURL,
		IsAzure:      true,
		APIVersion:   orDefault(cfg.AzureOpenAI.APIVersion, "2024-10-21"),
		DefaultModel: cfg.AzureOpenAI.DefaultModel,
	})}
}

func appendAnthropic(cfg *Config) []provider.Provider {
	if cfg.Anthropic == nil {
		return nil
	}
	return []provider.Provider{anthropic.New(anthropic.Config{
		APIKey:       cfg.Anthropic.APIKey,
		BaseURL:      cfg.Anthropic.BaseURL,
		DefaultModel: orDefault(cfg.Anthropic.DefaultModel, "claude-haiku-3-5"),
		BetaFeatures: cfg.Anthropic.BetaFeatures,
	})}
}

func appendOllama(cfg *Config) []provider.Provider {
	if cfg.Ollama == nil {
		return nil
	}
	return []provider.Provider{ollama.New(ollama.Config{
		BaseURL:      cfg.Ollama.BaseURL,
		DefaultModel: cfg.Ollama.DefaultModel,
	})}
}

func appendVLLM(cfg *Config) []provider.Provider {
	if cfg.VLLM == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "vllm",
		BaseURL:        orDefault(cfg.VLLM.BaseURL, "http://localhost:8000"),
		APIKey:         cfg.VLLM.APIKey,
		DefaultModel:   cfg.VLLM.DefaultModel,
		DiscoverModels: true,
	})}
}

func appendLMStudio(cfg *Config) []provider.Provider {
	if cfg.LMStudio == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "lmstudio",
		BaseURL:        orDefault(cfg.LMStudio.BaseURL, "http://localhost:1234"),
		DefaultModel:   cfg.LMStudio.DefaultModel,
		DiscoverModels: true,
	})}
}

func appendGroq(cfg *Config) []provider.Provider {
	if cfg.Groq == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "groq",
		BaseURL:      "https://api.groq.com/openai/v1",
		APIKey:       cfg.Groq.APIKey,
		DefaultModel: orDefault(cfg.Groq.DefaultModel, "llama-3.3-70b-versatile"),
		StaticModels: groqModels(),
	})}
}

func appendMistral(cfg *Config) []provider.Provider {
	if cfg.Mistral == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "mistral",
		BaseURL:      "https://api.mistral.ai",
		APIKey:       cfg.Mistral.APIKey,
		DefaultModel: orDefault(cfg.Mistral.DefaultModel, "mistral-small-latest"),
		StaticModels: mistralModels(),
	})}
}

func appendTogether(cfg *Config) []provider.Provider {
	if cfg.Together == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "together",
		BaseURL:        "https://api.together.xyz",
		APIKey:         cfg.Together.APIKey,
		DefaultModel:   cfg.Together.DefaultModel,
		DiscoverModels: true,
	})}
}

func appendOpenRouter(cfg *Config) []provider.Provider {
	if cfg.OpenRouter == nil {
		return nil
	}
	extraHeaders := map[string]string{
		"HTTP-Referer": cfg.OpenRouter.SiteURL,
		"X-Title":      cfg.OpenRouter.SiteName,
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "openrouter",
		BaseURL:        "https://openrouter.ai/api",
		APIKey:         cfg.OpenRouter.APIKey,
		DefaultModel:   cfg.OpenRouter.DefaultModel,
		ExtraHeaders:   extraHeaders,
		DiscoverModels: true,
	})}
}

func appendNVIDIA(cfg *Config) []provider.Provider {
	if cfg.NVIDIA == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "nvidia",
		BaseURL:      "https://integrate.api.nvidia.com",
		APIKey:       cfg.NVIDIA.APIKey,
		DefaultModel: orDefault(cfg.NVIDIA.DefaultModel, "nvidia/llama-3.1-nemotron-70b-instruct"),
		StaticModels: nvidiaModels(),
	})}
}

func appendCohere(cfg *Config) []provider.Provider {
	if cfg.Cohere == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "cohere",
		BaseURL:      "https://api.cohere.com",
		APIKey:       cfg.Cohere.APIKey,
		DefaultModel: orDefault(cfg.Cohere.DefaultModel, "command-r-plus-08-2024"),
		StaticModels: cohereModels(),
	})}
}

func appendHuggingFace(cfg *Config) []provider.Provider {
	if cfg.HuggingFace == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "huggingface",
		BaseURL:        orDefault(cfg.HuggingFace.BaseURL, "https://api-inference.huggingface.co"),
		APIKey:         cfg.HuggingFace.APIKey,
		DefaultModel:   cfg.HuggingFace.DefaultModel,
		DiscoverModels: false,
	})}
}

func appendDeepSeek(cfg *Config) []provider.Provider {
	if cfg.DeepSeek == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:           "deepseek",
		BaseURL:        "https://api.deepseek.com",
		APIKey:         cfg.DeepSeek.APIKey,
		DefaultModel:   orDefault(cfg.DeepSeek.DefaultModel, "deepseek-chat"),
		DiscoverModels: true,
	})}
}

func appendPerplexity(cfg *Config) []provider.Provider {
	if cfg.Perplexity == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "perplexity",
		BaseURL:      "https://api.perplexity.ai",
		APIKey:       cfg.Perplexity.APIKey,
		DefaultModel: orDefault(cfg.Perplexity.DefaultModel, "sonar-reasoning-pro"),
		StaticModels: perplexityModels(),
	})}
}

func appendXAI(cfg *Config) []provider.Provider {
	if cfg.XAI == nil {
		return nil
	}
	return []provider.Provider{openaicompat.New(openaicompat.Config{
		Name:         "xai",
		BaseURL:      "https://api.x.ai/v1",
		APIKey:       cfg.XAI.APIKey,
		DefaultModel: orDefault(cfg.XAI.DefaultModel, "grok-3"),
		StaticModels: xaiModels(),
	})}
}

// Build creates all configured providers. Extend first-party wiring via
// [providerConfigBuilders], or out-of-tree via [RegisterConfigBuilder].
func Build(cfg *Config) []provider.Provider {
	var out []provider.Provider
	for _, b := range providerConfigBuilders {
		out = append(out, b(cfg)...)
	}
	extraBuildersMu.Lock()
	ext := append([]ConfigBuilder(nil), extraBuilders...)
	extraBuildersMu.Unlock()
	for _, b := range ext {
		out = append(out, b(cfg)...)
	}
	return out
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// ─────────────────────────────────────────────
// Static model catalogs for providers that
// don't expose reliable /v1/models endpoints
// ─────────────────────────────────────────────

func groqModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
	return []provider.ModelInfo{
		{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B", Provider: "groq", Capabilities: caps, ContextWindow: 128000, MaxOutputTokens: 32768},
		{ID: "llama-3.1-8b-instant", Name: "Llama 3.1 8B Instant", Provider: "groq", Capabilities: caps, ContextWindow: 128000, MaxOutputTokens: 8192},
		{ID: "mixtral-8x7b-32768", Name: "Mixtral 8x7B", Provider: "groq", Capabilities: caps, ContextWindow: 32768, MaxOutputTokens: 32768},
		{ID: "qwen-2.5-coder-32b", Name: "Qwen 2.5 Coder 32B", Provider: "groq", Capabilities: caps, ContextWindow: 128000, MaxOutputTokens: 8192},
		{ID: "gemma2-9b-it", Name: "Gemma 2 9B", Provider: "groq", Capabilities: caps, ContextWindow: 8192, MaxOutputTokens: 8192},
		{ID: "deepseek-r1-distill-llama-70b", Name: "DeepSeek R1 Distill 70B", Provider: "groq", Capabilities: append(caps, provider.CapabilityReasoning), ContextWindow: 128000, MaxOutputTokens: 16384},
	}
}

func mistralModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
	vision := append(caps, provider.CapabilityVision)
	return []provider.ModelInfo{
		{ID: "mistral-large-latest", Name: "Mistral Large", Provider: "mistral", Capabilities: caps, ContextWindow: 128000, InputCostPerM: 2, OutputCostPerM: 6},
		{ID: "mistral-small-latest", Name: "Mistral Small", Provider: "mistral", Capabilities: caps, ContextWindow: 128000, InputCostPerM: 0.1, OutputCostPerM: 0.3},
		{ID: "codestral-latest", Name: "Codestral", Provider: "mistral", Capabilities: caps, ContextWindow: 256000, InputCostPerM: 0.3, OutputCostPerM: 0.9},
		{ID: "pixtral-large-latest", Name: "Pixtral Large", Provider: "mistral", Capabilities: vision, ContextWindow: 128000, InputCostPerM: 2, OutputCostPerM: 6},
		{ID: "mistral-saba-latest", Name: "Mistral Saba", Provider: "mistral", Capabilities: caps, ContextWindow: 32000},
	}
}

func nvidiaModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
	return []provider.ModelInfo{
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Name: "NVIDIA Nemotron 70B", Provider: "nvidia", Capabilities: caps, ContextWindow: 131072},
		{ID: "meta/llama-3.3-70b-instruct", Name: "Llama 3.3 70B", Provider: "nvidia", Capabilities: caps, ContextWindow: 131072},
		{ID: "nvidia/mistral-nemo-minitron-8b-8k-instruct", Name: "NeMo Minitron 8B", Provider: "nvidia", Capabilities: caps, ContextWindow: 8192},
	}
}

func cohereModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
	return []provider.ModelInfo{
		{ID: "command-r-plus-08-2024", Name: "Command R+ (Aug 2024)", Provider: "cohere", Capabilities: caps, ContextWindow: 128000, InputCostPerM: 2.5, OutputCostPerM: 10},
		{ID: "command-r-08-2024", Name: "Command R (Aug 2024)", Provider: "cohere", Capabilities: caps, ContextWindow: 128000, InputCostPerM: 0.15, OutputCostPerM: 0.6},
		{ID: "command-a-03-2025", Name: "Command A", Provider: "cohere", Capabilities: caps, ContextWindow: 256000},
	}
}

func perplexityModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming}
	reason := append(caps, provider.CapabilityReasoning)
	return []provider.ModelInfo{
		{ID: "sonar-reasoning-pro", Name: "Sonar Reasoning Pro", Provider: "perplexity", Capabilities: reason, ContextWindow: 128000},
		{ID: "sonar-reasoning", Name: "Sonar Reasoning", Provider: "perplexity", Capabilities: reason, ContextWindow: 128000},
		{ID: "sonar-pro", Name: "Sonar Pro", Provider: "perplexity", Capabilities: caps, ContextWindow: 128000},
		{ID: "sonar", Name: "Sonar", Provider: "perplexity", Capabilities: caps, ContextWindow: 128000},
	}
}

func xaiModels() []provider.ModelInfo {
	caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
	vision := append(caps, provider.CapabilityVision)
	return []provider.ModelInfo{
		{ID: "grok-3", Name: "Grok 3", Provider: "xai", Capabilities: caps, ContextWindow: 131072},
		{ID: "grok-3-vision", Name: "Grok 3 Vision", Provider: "xai", Capabilities: vision, ContextWindow: 32768},
		{ID: "grok-2", Name: "Grok 2", Provider: "xai", Capabilities: caps, ContextWindow: 131072},
		{ID: "grok-2-vision", Name: "Grok 2 Vision", Provider: "xai", Capabilities: vision, ContextWindow: 32768},
		{ID: "grok-2-1212", Name: "Grok 2 (Dec 2024)", Provider: "xai", Capabilities: caps, ContextWindow: 131072},
	}
}
