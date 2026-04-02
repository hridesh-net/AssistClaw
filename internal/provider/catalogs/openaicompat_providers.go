package catalogs

import "github.com/assistclaw/assistclaw/internal/provider"

// GroqModels is a curated list (API discovery still augments when enabled).
func GroqModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct {
		id, name string
		ctx, max int
	}{
		{"llama-3.3-70b-versatile", "Llama 3.3 70B Versatile", 131072, 32768},
		{"llama-3.1-8b-instant", "Llama 3.1 8B Instant", 131072, 8192},
		{"llama-3.1-70b-versatile", "Llama 3.1 70B Versatile", 131072, 8192},
		{"llama3-70b-8192", "Llama 3 70B", 8192, 8192},
		{"llama3-8b-8192", "Llama 3 8B", 8192, 8192},
		{"mixtral-8x7b-32768", "Mixtral 8x7B", 32768, 32768},
		{"gemma2-9b-it", "Gemma 2 9B IT", 8192, 8192},
	}
	return modelRows(prov, ts, rows)
}

// MistralModels curated list for api.mistral.ai.
func MistralModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	tv := append(append([]provider.Capability{}, ts...), provider.CapabilityVision)
	return []provider.ModelInfo{
		{ID: "mistral-large-latest", Name: "Mistral Large Latest", Provider: prov, Capabilities: ts, ContextWindow: 131072, MaxOutputTokens: 8192},
		{ID: "mistral-small-latest", Name: "Mistral Small Latest", Provider: prov, Capabilities: ts, ContextWindow: 32000, MaxOutputTokens: 8192},
		{ID: "ministral-8b-latest", Name: "Ministral 8B Latest", Provider: prov, Capabilities: ts, ContextWindow: 131072, MaxOutputTokens: 8192},
		{ID: "codestral-latest", Name: "Codestral Latest", Provider: prov, Capabilities: ts, ContextWindow: 256000, MaxOutputTokens: 8192},
		{ID: "pixtral-large-latest", Name: "Pixtral Large Latest", Provider: prov, Capabilities: tv, ContextWindow: 131072, MaxOutputTokens: 8192},
	}
}

// DeepSeekModels for api.deepseek.com.
func DeepSeekModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"deepseek-chat", "DeepSeek Chat", 64000, 8192},
		{"deepseek-reasoner", "DeepSeek Reasoner (R1)", 64000, 8192},
	}
	return modelRows(prov, ts, rows)
}

// PerplexityModels for api.perplexity.ai (OpenAI-compatible).
func PerplexityModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"sonar", "Sonar", 127072, 8192},
		{"sonar-pro", "Sonar Pro", 200000, 8192},
		{"sonar-reasoning", "Sonar Reasoning", 127072, 8192},
		{"sonar-reasoning-pro", "Sonar Reasoning Pro", 127072, 8192},
	}
	return modelRows(prov, ts, rows)
}

// NVIDIAModels for integrate.api.nvidia.com (curated; discovery augments).
func NVIDIAModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"meta/llama-3.1-405b-instruct", "Llama 3.1 405B Instruct", 131072, 8192},
		{"meta/llama-3.1-70b-instruct", "Llama 3.1 70B Instruct", 131072, 8192},
		{"meta/llama-3.1-8b-instruct", "Llama 3.1 8B Instruct", 131072, 8192},
		{"mistralai/mixtral-8x7b-instruct-v0.1", "Mixtral 8x7B Instruct", 32768, 8192},
	}
	return modelRows(prov, ts, rows)
}

// CohereModels for api.cohere.com v2-compatible chat.
func CohereModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"command-r-plus", "Command R+", 128000, 4096},
		{"command-r", "Command R", 128000, 4096},
		{"command-a-reasoning-08-2025", "Command A Reasoning", 256000, 8192},
	}
	return modelRows(prov, ts, rows)
}

// TogetherModels fallback list when /v1/models is empty.
func TogetherModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"meta-llama/Llama-3.3-70B-Instruct-Turbo", "Llama 3.3 70B Instruct Turbo", 131072, 8192},
		{"meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo", "Llama 3.1 405B Instruct Turbo", 131072, 8192},
		{"Qwen/Qwen2.5-72B-Instruct-Turbo", "Qwen 2.5 72B Instruct Turbo", 131072, 8192},
	}
	return modelRows(prov, ts, rows)
}

// OpenRouterModels fallback rows when the API returns few models.
func OpenRouterModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	rows := []struct{ id, name string; ctx, max int }{
		{"openai/gpt-4o", "OpenAI GPT-4o", 128000, 16384},
		{"openai/gpt-4o-mini", "OpenAI GPT-4o Mini", 128000, 16384},
		{"anthropic/claude-3.5-sonnet", "Anthropic Claude 3.5 Sonnet", 200000, 8192},
		{"google/gemini-pro-1.5", "Google Gemini Pro 1.5", 1000000, 8192},
		{"meta-llama/llama-3.3-70b-instruct", "Meta Llama 3.3 70B Instruct", 131072, 8192},
	}
	return modelRows(prov, ts, rows)
}

func modelRows(prov string, caps []provider.Capability, rows []struct {
	id, name string
	ctx, max int
}) []provider.ModelInfo {
	out := make([]provider.ModelInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, provider.ModelInfo{
			ID: r.id, Name: r.name, Provider: prov,
			Capabilities: caps, ContextWindow: r.ctx, MaxOutputTokens: r.max,
		})
	}
	return out
}
