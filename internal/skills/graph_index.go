package skills

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// SkillGraphIndexTool returns a compact index of all active skill graphs.
// The agent calls this first to discover which skills and nodes are available,
// then reads specific nodes via read_skill_node.
type SkillGraphIndexTool struct {
	Registry     Registry
	ActiveSkills []string // the skill names configured for current session
}

func (t *SkillGraphIndexTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "skill_graph_index",
		Description: `Returns a compact index of all active skill graphs with their available nodes.
Use this first when you need to use a skill but don't know which node to read.
The index shows node names as [[wikilinks]] that you can follow with read_skill_node.
Example output:
  github: [[SKILL]], [[create-pr]], [[review-pr]]
  notion: [[SKILL]], [[create-page]]`,
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		},
	}
}

func (t *SkillGraphIndexTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	if t.Registry == nil || len(t.ActiveSkills) == 0 {
		return "No active skills configured.", nil
	}

	var parts []string
	for _, name := range t.ActiveSkills {
		skill, ok := t.Registry.Get(name)
		if !ok {
			continue
		}

		var nodeRefs []string
		// SKILL node first
		if _, hasEntry := skill.Nodes["SKILL"]; hasEntry {
			nodeRefs = append(nodeRefs, "[[SKILL]]")
		}
		for nodeName := range skill.Nodes {
			if nodeName == "SKILL" {
				continue
			}
			nodeRefs = append(nodeRefs, "[["+nodeName+"]]")
		}

		if len(nodeRefs) == 0 {
			continue
		}
		parts = append(parts, name+": "+strings.Join(nodeRefs, ", "))
	}

	if len(parts) == 0 {
		return "No skill nodes found.", nil
	}

	result := strings.Join(parts, "\n")
	result += "\n\nCall read_skill_node(skill_name, node_name) to read any node's full content."
	return result, nil
}
