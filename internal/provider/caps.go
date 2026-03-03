package provider

// ProviderCaps describes capability constraints for a specific LLM provider.
// Used by the Catalog to decide how many and which tools to send per request.
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

// CapsFor returns the known capability profile for a named provider.
// The providerName should be the lowercase ID used in assistclaw.yaml (e.g. "anthropic", "bedrock").
func CapsFor(providerName string) ProviderCaps {
	switch providerName {
	case "anthropic", "claude":
		return ProviderCaps{
			MaxTools:           0,
			RequiresAllTools:   false,
			NativeToolUse:      true,
			SupportsToolResult: true,
		}
	case "openai", "gpt":
		return ProviderCaps{
			MaxTools:           128,
			RequiresAllTools:   false,
			NativeToolUse:      true,
			SupportsToolResult: true,
		}
	case "bedrock", "aws":
		return ProviderCaps{
			MaxTools:           0,
			RequiresAllTools:   true, // Bedrock needs full schema upfront
			NativeToolUse:      true,
			SupportsToolResult: true,
		}
	case "gemini", "vertex", "google":
		return ProviderCaps{
			MaxTools:           0,
			RequiresAllTools:   false,
			NativeToolUse:      true,
			SupportsToolResult: true,
		}
	case "ollama", "local":
		return ProviderCaps{
			MaxTools:           10, // local models struggle with too many tools
			RequiresAllTools:   false,
			NativeToolUse:      false, // treat as text-based unless proven otherwise
			SupportsToolResult: true,
		}
	default:
		// Conservative default: send a capped set, use text fallback
		return ProviderCaps{
			MaxTools:           12,
			RequiresAllTools:   false,
			NativeToolUse:      true,
			SupportsToolResult: true,
		}
	}
}
