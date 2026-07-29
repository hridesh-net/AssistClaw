package core

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
