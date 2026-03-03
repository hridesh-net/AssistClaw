package tools

import (
	"context"
	"encoding/json"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// FindToolsTool lets the agent discover available tools by keyword query.
// Returns plain text — works with any LLM, even without native tool-use support.
// This is the safety net that ensures no tool is ever truly hidden.
type FindToolsTool struct {
	Catalog *Catalog
}

func (t FindToolsTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "find_tools",
		Description: `Discover available tools by keyword search. Use this when you are unsure which tool handles a task.
Returns a plain-text list of matching tools with descriptions. Always available regardless of which tools are pre-loaded.
Examples:
  find_tools(query="schedule recurring task")   → returns: cron, bash, process
  find_tools(query="send message to user")      → returns: message
  find_tools(query="web_search")                → returns full description of web_search`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Keywords describing the capability you need, or an exact tool name for its full description.",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (t FindToolsTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if t.Catalog == nil {
		return "find_tools: catalog not initialised", nil
	}
	return t.Catalog.FindTools(args.Query, 5), nil
}
