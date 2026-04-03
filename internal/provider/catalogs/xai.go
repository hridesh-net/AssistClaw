// Package catalogs holds static LLM model lists aligned with common provider catalog layouts.
package catalogs

import "github.com/assistclaw/assistclaw/internal/provider"

// XAIModels lists known x.ai Grok models (refresh periodically against provider docs).
func XAIModels(prov string) []provider.ModelInfo {
	ts := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	tv := append(append([]provider.Capability{}, ts...), provider.CapabilityVision)

	type row struct {
		id, name string
		ctx, max int
		caps     []provider.Capability
	}
	rows := []row{
		{"grok-3", "Grok 3", 131072, 8192, ts},
		{"grok-3-fast", "Grok 3 Fast", 131072, 8192, ts},
		{"grok-3-mini", "Grok 3 Mini", 131072, 8192, ts},
		{"grok-3-mini-fast", "Grok 3 Mini Fast", 131072, 8192, ts},
		{"grok-4", "Grok 4", 256000, 64000, ts},
		{"grok-4-0709", "Grok 4 0709", 256000, 64000, ts},
		{"grok-4-fast", "Grok 4 Fast", 2000000, 30000, tv},
		{"grok-4-fast-non-reasoning", "Grok 4 Fast (Non-Reasoning)", 2000000, 30000, tv},
		{"grok-4-1-fast", "Grok 4.1 Fast", 2000000, 30000, tv},
		{"grok-4-1-fast-non-reasoning", "Grok 4.1 Fast (Non-Reasoning)", 2000000, 30000, tv},
		{"grok-4.20-beta-latest-reasoning", "Grok 4.20 Beta Latest (Reasoning)", 2000000, 30000, tv},
		{"grok-4.20-beta-latest-non-reasoning", "Grok 4.20 Beta Latest (Non-Reasoning)", 2000000, 30000, tv},
		{"grok-code-fast-1", "Grok Code Fast 1", 256000, 10000, ts},
	}
	out := make([]provider.ModelInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, provider.ModelInfo{
			ID: r.id, Name: r.name, Provider: prov,
			Capabilities: r.caps, ContextWindow: r.ctx, MaxOutputTokens: r.max,
		})
	}
	return out
}
