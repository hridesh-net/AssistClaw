package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// ReadSkillNodeTool allows the agent to lazily load a specific node from a Skill Graph.
type ReadSkillNodeTool struct {
	registry Registry
}

var _ agent.Tool = (*ReadSkillNodeTool)(nil)

func NewReadSkillNodeTool(registry Registry) *ReadSkillNodeTool {
	return &ReadSkillNodeTool{registry: registry}
}

func (t *ReadSkillNodeTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "read_skill_node",
		Description: "Reads the full content of a specific node within a Skill Graph. Use this when the 'Map of Content' suggests a node contains relevant instructions or logic.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"skill_name": provider.ToolParameter{
					Type:        "string",
					Description: "The name of the skill graph (e.g. 'skill-creator')",
				},
				"node_name": provider.ToolParameter{
					Type:        "string",
					Description: "The name of the node to read (e.g. 'init_skill' or 'SKILL')",
				},
			},
			Required: []string{"skill_name", "node_name"},
		},
	}
}

func (t *ReadSkillNodeTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		SkillName string `json:"skill_name"`
		NodeName  string `json:"node_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	node, ok := t.registry.ReadSkillNode(args.SkillName, args.NodeName)
	if !ok {
		return "", fmt.Errorf("skill node '%s/%s' not found", args.SkillName, args.NodeName)
	}

	return fmt.Sprintf("## Node: %s/%s\n\n%s", args.SkillName, node.Name, node.Instructions), nil
}
