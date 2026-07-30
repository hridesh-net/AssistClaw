package core

import (
	"context"
	"encoding/json"
)

// ─────────────────────────────────────────────
// Tool contract
// ─────────────────────────────────────────────

// Tool is the interface that all built-in and user-generated tools must implement.
type Tool interface {
	// Definition returns the schema passed to the LLM.
	Definition() ToolDef
	// Execute runs the tool with the given JSON input.
	// Returns (output string, error). Non-fatal errors should be returned
	// as output strings so the LLM can reason about them.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// ToolRegistry maps tool names to implementations.
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool.
func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Definition().Name] = t
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns all tool definitions for LLM requests.
func (r *ToolRegistry) Definitions() []ToolDef {
	defs := make([]ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// CloneWithout returns a new registry with the given tool names omitted (e.g. avoid sub-agent recursion).
func (r *ToolRegistry) CloneWithout(omit ...string) *ToolRegistry {
	skip := make(map[string]bool, len(omit))
	for _, n := range omit {
		skip[n] = true
	}
	out := NewToolRegistry()
	for _, t := range r.tools {
		n := t.Definition().Name
		if skip[n] {
			continue
		}
		out.Register(t)
	}
	return out
}

// ToolCatalog is the interface the runner uses to select per-request tools.
// Implemented by tools.Catalog. If nil, callers fall back to ToolRegistry.Definitions().
type ToolCatalog interface {
	SelectForRequest(userMessage string, caps ProviderCaps) []ToolDef
	RecordUsage(toolName string)
	DecayInertia()
}
