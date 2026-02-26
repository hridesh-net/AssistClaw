package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/skills"
	"go.uber.org/zap"
)

// ─────────────────────────────────────────────
// RegisterExternalMCPTools
// ─────────────────────────────────────────────

// RegisterExternalMCPTools connects to all configured external MCP servers,
// discovers their tools, and registers them in:
//  1. The agent's tool registry (so the agent can call them directly)
//  2. The MCP server's handler map (so external clients can proxy through them)
//  3. The skill registry as virtual skill nodes (so they appear in the Map of Content)
func RegisterExternalMCPTools(
	ctx context.Context,
	configs []ClientConfig,
	skillReg skills.Registry,
	toolReg *agent.ToolRegistry,
	mcpSrv *Server,
	log *zap.Logger,
) {
	for _, cfg := range configs {
		cfg := cfg
		client := NewClient(cfg, log)
		if err := client.Connect(ctx); err != nil {
			log.Warn("mcp: failed to connect to external server",
				zap.String("name", cfg.Name),
				zap.Error(err),
			)
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			log.Warn("mcp: failed to list tools",
				zap.String("name", cfg.Name),
				zap.Error(err),
			)
			_ = client.Close()
			continue
		}

		log.Info("mcp: registered external server tools",
			zap.String("server", cfg.Name),
			zap.Int("tools", len(tools)),
		)

		// Register each remote tool as an agent tool + MCP handler
		for _, rt := range tools {
			rt := rt
			toolName := "mcp:" + rt.ServerName + ":" + rt.Definition.Name
			agentTool := &mcpAgentTool{
				name:        toolName,
				description: rt.Definition.Description,
				schema:      rt.Definition.InputSchema,
				call:        rt.Call,
			}
			toolReg.Register(agentTool)
			// Also expose through MCP server so external clients can call it
			if mcpSrv != nil {
				mcpSrv.RegisterTool(toolName, func(ctx context.Context, args map[string]any) (CallToolResult, error) {
					return rt.Call(ctx, args)
				})
			}
		}

		// Register the whole MCP server as a virtual skill node in the skill graph
		// so it appears in the Map of Content with all sub-tools as nodes
		virtualSkill := buildVirtualSkill(cfg.Name, tools)
		skillReg.Register(virtualSkill)

		// Register node-level read handlers on the MCP server
		if mcpSrv != nil {
			registerMCPNodeHandlers(mcpSrv, cfg.Name, tools)
		}
	}
}

// buildVirtualSkill creates a synthetic Skill from a set of remote MCP tools.
// The skill root summarizes the server; each tool becomes a Node with full spec.
func buildVirtualSkill(serverName string, tools []RemoteTool) *skills.Skill {
	sk := &skills.Skill{
		Name:        "mcp:" + serverName,
		Description: fmt.Sprintf("External MCP server %q — %d tools available", serverName, len(tools)),
		Nodes:       make(map[string]*skills.Node),
	}

	// Root node: list all tools + usage hint
	var rootSB strings.Builder
	rootSB.WriteString(fmt.Sprintf("# MCP Server: %s\n\n", serverName))
	rootSB.WriteString(fmt.Sprintf("%d tools available. Use `read_skill_node` to get full specs.\n\n", len(tools)))

	// Group tools by category prefix (e.g. "read_file" → group "read")
	groups := groupTools(tools)

	for groupName, groupTools := range groups {
		rootSB.WriteString(fmt.Sprintf("## %s\n", groupName))
		for _, t := range groupTools {
			rootSB.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Definition.Name, t.Definition.Description))
		}
		rootSB.WriteString("\n")

		// Create a group node
		var groupSB strings.Builder
		groupSB.WriteString(fmt.Sprintf("# %s / %s tools\n\n", serverName, groupName))
		for _, t := range groupTools {
			groupSB.WriteString(fmt.Sprintf("## %s\n%s\n\n", t.Definition.Name, t.Definition.Description))
			groupSB.WriteString(formatInputSchema(t.Definition.InputSchema))
		}

		sk.Nodes[groupName] = &skills.Node{
			Name:         groupName,
			Summary:      fmt.Sprintf("%d %s tools", len(groupTools), groupName),
			Instructions: groupSB.String(),
		}
	}

	// Also add individual tool nodes for direct access
	for _, t := range tools {
		var toolSB strings.Builder
		toolSB.WriteString(fmt.Sprintf("# %s\n\n%s\n\n", t.Definition.Name, t.Definition.Description))
		toolSB.WriteString(formatInputSchema(t.Definition.InputSchema))
		toolSB.WriteString(fmt.Sprintf("\nCall: `mcp:%s:%s` with the above parameters.\n", t.ServerName, t.Definition.Name))

		sk.Nodes[t.Definition.Name] = &skills.Node{
			Name:         t.Definition.Name,
			Summary:      t.Definition.Description,
			Instructions: toolSB.String(),
		}
	}

	sk.Nodes["SKILL"] = &skills.Node{
		Name:         "SKILL",
		Summary:      sk.Description,
		Instructions: rootSB.String(),
	}

	return sk
}

// groupTools clusters tools by a common prefix word for tidy Map of Content.
func groupTools(tools []RemoteTool) map[string][]RemoteTool {
	groups := make(map[string][]RemoteTool)
	for _, t := range tools {
		parts := strings.SplitN(t.Definition.Name, "_", 2)
		group := parts[0]
		if len(tools) <= 8 {
			// Small tool sets: put everything in "tools" group (no need to split)
			group = "tools"
		}
		groups[group] = append(groups[group], t)
	}
	return groups
}

// formatInputSchema converts an InputSchema into markdown for human/LLM reading.
func formatInputSchema(schema InputSchema) string {
	if len(schema.Properties) == 0 {
		return "Parameters: none\n"
	}
	var sb strings.Builder
	sb.WriteString("### Parameters\n")
	for name, prop := range schema.Properties {
		required := ""
		for _, r := range schema.Required {
			if r == name {
				required = " *(required)*"
				break
			}
		}
		sb.WriteString(fmt.Sprintf("- **%s** (`%s`)%s: %s\n", name, prop.Type, required, prop.Description))
	}
	return sb.String()
}

// registerMCPNodeHandlers registers read_skill_node-style handlers on the MCP server
// so that `read_skill_node(skill="mcp:filesystem", node="read_file")` works.
func registerMCPNodeHandlers(srv *Server, serverName string, tools []RemoteTool) {
	for _, t := range tools {
		t := t
		key := "mcp:node:" + serverName + "/" + t.Definition.Name
		nodeTool := srv // capture srv
		_ = nodeTool
		srv.RegisterTool(key, func(ctx context.Context, args map[string]any) (CallToolResult, error) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("# %s\n\n%s\n\n", t.Definition.Name, t.Definition.Description))
			sb.WriteString(formatInputSchema(t.Definition.InputSchema))
			sb.WriteString(fmt.Sprintf("\nTo call this tool use: `mcp:%s:%s`\n", serverName, t.Definition.Name))
			return textResult(sb.String()), nil
		})
	}
}

// ─────────────────────────────────────────────
// agent.Tool adapter
// ─────────────────────────────────────────────

// mcpAgentTool wraps a RemoteTool as an agent.Tool so the agent can call
// external MCP tools the same way it calls built-in tools.
type mcpAgentTool struct {
	name        string
	description string
	schema      InputSchema
	call        func(ctx context.Context, args map[string]any) (CallToolResult, error)
}

func (t *mcpAgentTool) Definition() provider.ToolDef {
	props := make(map[string]any, len(t.schema.Properties))
	for k, v := range t.schema.Properties {
		props[k] = map[string]any{
			"type":        v.Type,
			"description": v.Description,
		}
	}
	return provider.ToolDef{
		Name:        t.name,
		Description: t.description,
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: props,
			Required:   t.schema.Required,
		},
	}
}

func (t *mcpAgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("mcp tool %s: invalid input: %w", t.name, err)
	}
	result, err := t.call(ctx, args)
	if err != nil {
		return "", err
	}
	if result.IsError {
		var texts []string
		for _, c := range result.Content {
			texts = append(texts, c.Text)
		}
		return "", fmt.Errorf("mcp tool error: %s", strings.Join(texts, "\n"))
	}
	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// ensure mcpAgentTool satisfies agent.Tool at compile time.
var _ agent.Tool = (*mcpAgentTool)(nil)
