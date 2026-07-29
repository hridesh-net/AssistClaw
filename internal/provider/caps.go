package provider

import "github.com/assistclaw/assistclaw/internal/core"

// ProviderCaps describes capability constraints for a specific LLM provider.
// The struct now lives in internal/core; this alias keeps provider.ProviderCaps
// working for existing references.
type ProviderCaps = core.ProviderCaps

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
