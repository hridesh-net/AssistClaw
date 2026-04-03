package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// DynamicTool wraps a skill-defined script into an agent.Tool.
type DynamicTool struct {
	skillTool SkillTool
	workDir   string
}

// Ensure DynamicTool implements agent.Tool
var _ agent.Tool = (*DynamicTool)(nil)

func (t *DynamicTool) Definition() provider.ToolDef {
	var params provider.ToolParameter
	if len(t.skillTool.Schema) > 0 {
		_ = json.Unmarshal(t.skillTool.Schema, &params)
	}
	return provider.ToolDef{
		Name:        t.skillTool.Name,
		Description: t.skillTool.Description,
		InputSchema: params,
	}
}

func (t *DynamicTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// The command defined in SKILL.md (e.g., "python3 fetch_data.py")
	parts := strings.Fields(t.skillTool.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = t.workDir

	// Provide the input as STDIN (most bundled skill scripts expect this)
	cmd.Stdin = bytes.NewReader(input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// We return the output string alongside the error so the agent sees the stderr output
		return string(output), fmt.Errorf("skill tool execution failed: %w", err)
	}

	return string(output), nil
}

// ConvertTools takes a Skill and returns a slice of agent.Tools for it.
func ConvertTools(skill *Skill, skillDir string) []agent.Tool {
	var out []agent.Tool
	for _, st := range skill.Tools {
		out = append(out, &DynamicTool{
			skillTool: st,
			workDir:   skillDir,
		})
	}
	return out
}
