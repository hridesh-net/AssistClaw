package tools

import (
	"context"
	"encoding/json"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// FinishTaskTool is used by the agent in autonomous mode to signal that it has completed its high-level goal.
type FinishTaskTool struct{}

func (FinishTaskTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "finish_task",
		Description: "Signal that you have completely finished the overarching goal or task. Call this ONLY when you are completely done.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"reason": map[string]any{"type": "string", "description": "Summary of what was accomplished"},
			},
			Required: []string{"reason"},
		},
	}
}

func (FinishTaskTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "Finished without reason.", nil
	}
	return "Task marked as finished: " + args.Reason, nil
}
