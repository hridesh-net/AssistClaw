package provider

// ProviderCaps describes capability constraints for a specific LLM provider.
// Used by the catalog to decide how many and which tools to send per request.
type ProviderCaps struct {
	// MaxTools is the practical limit on simultaneous tool definitions (0 = no limit).
	MaxTools int
	// RequiresAllTools means the provider needs ALL tools declared in the first request
	// and does not support per-request subsetting (e.g. AWS Bedrock).
	RequiresAllTools bool
	// NativeToolUse indicates the provider has a first-class tool-calling API
	// (JSON schema input/output). If false, tools are simulated via text.
	NativeToolUse bool
	// SupportsToolResult indicates the provider can receive tool results back
	// in a follow-up message (all major providers do, some older ones don't).
	SupportsToolResult bool
}

// Preset capability profiles for [Provider.Caps]. Central definitions avoid a
// string-based switch and let each backend own its policy via the interface.

// CapsAnthropic is the profile for Anthropic Messages API providers.
func CapsAnthropic() ProviderCaps {
	return ProviderCaps{
		MaxTools:           0,
		RequiresAllTools:   false,
		NativeToolUse:      true,
		SupportsToolResult: true,
	}
}

// CapsOpenAI is the profile for OpenAI Chat Completions (non-Azure).
func CapsOpenAI() ProviderCaps {
	return ProviderCaps{
		MaxTools:           128,
		RequiresAllTools:   false,
		NativeToolUse:      true,
		SupportsToolResult: true,
	}
}

// CapsBedrock is the profile for AWS Bedrock (full tool schema upfront).
func CapsBedrock() ProviderCaps {
	return ProviderCaps{
		MaxTools:           0,
		RequiresAllTools:   true,
		NativeToolUse:      true,
		SupportsToolResult: true,
	}
}

// CapsGemini is the profile for Google Gemini / Vertex AI.
func CapsGemini() ProviderCaps {
	return ProviderCaps{
		MaxTools:           0,
		RequiresAllTools:   false,
		NativeToolUse:      true,
		SupportsToolResult: true,
	}
}

// CapsOllama is the profile for local Ollama (conservative tool count, text-style tools).
func CapsOllama() ProviderCaps {
	return ProviderCaps{
		MaxTools:           10,
		RequiresAllTools:   false,
		NativeToolUse:      false,
		SupportsToolResult: true,
	}
}

// CapsOpenAICompatDefault is the conservative default for OpenAI-compatible HTTP backends
// (Groq, Mistral, vLLM, OpenRouter, etc.) when no stricter profile applies.
func CapsOpenAICompatDefault() ProviderCaps {
	return ProviderCaps{
		MaxTools:           12,
		RequiresAllTools:   false,
		NativeToolUse:      true,
		SupportsToolResult: true,
	}
}
