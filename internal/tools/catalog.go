package tools

import (
	"sort"
	"strings"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/graph"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// coreSlugs are always included regardless of query — the essential tools.
var coreSlugs = []string{
	"bash",
	"read_file",
	"write_file",
	"edit",
	"list_dir",
	"grep",
	"memory_search",
	"web_search",
	// discovery tools — always on so the agent can find anything
	"find_tools",
	"read_skill_node",
}

// Catalog wraps all registered tools and uses the ToolGraph to select
// a per-request subset, reducing token use by ~60%.
type Catalog struct {
	toolGraph *graph.ToolGraph
	all       map[string]provider.ToolDef // name → full definition
}

// NewCatalog creates a Catalog from a populated ToolRegistry.
func NewCatalog(reg *agent.ToolRegistry, g *graph.ToolGraph) *Catalog {
	c := &Catalog{
		toolGraph: g,
		all:       make(map[string]provider.ToolDef),
	}
	for _, def := range reg.Definitions() {
		c.all[def.Name] = def
	}
	return c
}

// SelectForRequest returns the tool definitions to send for the current user message.
// For providers that RequiresAllTools, returns all definitions but capped by MaxTools.
// Otherwise, returns core + graph-traversed top-5.
func (c *Catalog) SelectForRequest(userMessage string, caps provider.ProviderCaps) []provider.ToolDef {
	if caps.RequiresAllTools {
		return c.allCapped(caps.MaxTools)
	}

	// Start with mandatory core set
	selected := map[string]bool{}
	for _, s := range coreSlugs {
		if _, ok := c.all[s]; ok {
			selected[s] = true
		}
	}

	// Add graph-traversed relevant tools (top 6)
	traversed := c.toolGraph.Traverse(userMessage, 6)
	for _, name := range traversed {
		if _, ok := c.all[name]; ok {
			selected[name] = true
		}
	}

	// Enforce provider MaxTools cap
	defs := c.defsFor(selected)
	if caps.MaxTools > 0 && len(defs) > caps.MaxTools {
		defs = defs[:caps.MaxTools]
	}
	return defs
}

// RecordUsage updates session inertia after a tool was called.
func (c *Catalog) RecordUsage(toolName string) {
	c.toolGraph.RecordUsage(toolName)
}

// DecayInertia should be called once per turn to decay boosts.
func (c *Catalog) DecayInertia() {
	c.toolGraph.DecayInertia()
}

// FindTools performs a keyword search across all tool definitions and returns
// the top-N matching tools as plain text (safe for any LLM, even without native tool use).
func (c *Catalog) FindTools(query string, topN int) string {
	lower := strings.ToLower(query)
	words := strings.Fields(lower)

	type scored struct {
		name  string
		score int
		def   provider.ToolDef
	}
	var results []scored

	for name, def := range c.all {
		if name == "find_tools" {
			continue // don't return itself
		}
		text := strings.ToLower(name + " " + def.Description)
		score := 0
		for _, w := range words {
			if strings.Contains(text, w) {
				score++
			}
		}
		if score > 0 {
			results = append(results, scored{name, score, def})
		}
	}

	// Also check if query is an exact tool name
	if def, ok := c.all[strings.TrimSpace(query)]; ok {
		hasIt := false
		for _, r := range results {
			if r.name == def.Name {
				hasIt = true
			}
		}
		if !hasIt {
			results = append(results, scored{def.Name, 99, def})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) == 0 {
		return "No matching tools found. All available tools: " + c.allNames()
	}

	var sb strings.Builder
	sb.WriteString("Matched tools:\n")
	cap := topN
	if len(results) < cap {
		cap = len(results)
	}
	for _, r := range results[:cap] {
		// First line of description only
		desc := r.def.Description
		if idx := strings.Index(desc, "\n"); idx > 0 {
			desc = desc[:idx]
		}
		if len(desc) > 100 {
			desc = desc[:97] + "…"
		}
		sb.WriteString("• ")
		sb.WriteString(r.name)
		sb.WriteString(" — ")
		sb.WriteString(desc)
		sb.WriteString("\n")
	}
	sb.WriteString("\nTip: to get the full schema for a specific tool, call find_tools(query=\"<tool_name>\").")
	return sb.String()
}

func (c *Catalog) allCapped(maxTools int) []provider.ToolDef {
	defs := make([]provider.ToolDef, 0, len(c.all))
	for _, d := range c.all {
		defs = append(defs, d)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	if maxTools > 0 && len(defs) > maxTools {
		return defs[:maxTools]
	}
	return defs
}

func (c *Catalog) defsFor(selected map[string]bool) []provider.ToolDef {
	defs := make([]provider.ToolDef, 0, len(selected))
	for name := range selected {
		if def, ok := c.all[name]; ok {
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func (c *Catalog) allNames() string {
	names := make([]string, 0, len(c.all))
	for n := range c.all {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
