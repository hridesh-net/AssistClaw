package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// RepairSkillTool allows the agent to trigger automated dependency installation.
type RepairSkillTool struct {
	Registry Registry
}

func (t *RepairSkillTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "repair_skill",
		Description: `Attempts to automatically install missing dependencies (binaries, packages) for a specific skill.
After running this, call skill_graph_index to see if the skill is now active.
Only use this if the user asks you to fix a specific skill or if you see it listed as 'Unavailable' in the system prompt.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "The name of the skill to repair (e.g. 'github', 'clawhub')",
				},
			},
			Required: []string{"skill_name"},
		},
	}
}

func (t *RepairSkillTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		SkillName string `json:"skill_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	skill, ok := t.Registry.Get(args.SkillName)
	if !ok {
		return fmt.Sprintf("Skill %q not found.", args.SkillName), nil
	}

	err := t.Registry.InstallDependency(ctx, skill)
	if err != nil {
		return fmt.Sprintf("Failed to repair skill %q: %v", args.SkillName, err), nil
	}

	return fmt.Sprintf("Successfully repaired skill %q. It should now be available in skill_graph_index.", args.SkillName), nil
}
