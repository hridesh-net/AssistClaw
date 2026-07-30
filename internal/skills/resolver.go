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
	skillTool   SkillTool
	workDir     string
	skillName   string
	sensitive   bool
	approveFunc SensitiveSkillApprover
}

// SensitiveSkillApprover decides whether a sensitive skill tool may run.
// Implementations typically check an in-memory allow-list populated from
// CLI flags or an interactive prompt. The runner injects one of these via
// SetApprover so tests can swap the policy.
type SensitiveSkillApprover func(skillName, toolName string) bool

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

// SkillName returns the parent skill name. Used by the runner to find a
// tool's owning skill when surfacing audit entries.
func (t *DynamicTool) SkillName() string { return t.skillName }

// IsSensitive returns true when the parent skill is marked sensitive.
func (t *DynamicTool) IsSensitive() bool { return t.sensitive }

func (t *DynamicTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.sensitive {
		// Default-deny: if no approver was attached, refuse rather than
		// silently running a sensitive skill (e.g. secret retrieval).
		if t.approveFunc == nil || !t.approveFunc(t.skillName, t.skillTool.Name) {
			return "", fmt.Errorf("skill %q is marked sensitive; pass --allow-sensitive-skills or add %q under agent.enabled_sensitive_skills in your config to enable",
				t.skillName, t.skillName)
		}
	}

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
// `approver` is consulted before any sensitive tool runs; pass nil to keep
// the default-deny policy (sensitive tools refuse to execute).
func ConvertTools(skill *Skill, skillDir string, approver SensitiveSkillApprover) []agent.Tool {
	var out []agent.Tool
	for _, st := range skill.Tools {
		out = append(out, &DynamicTool{
			skillTool:   st,
			workDir:     skillDir,
			skillName:   skill.Name,
			sensitive:   skill.Sensitive,
			approveFunc: approver,
		})
	}
	return out
}
