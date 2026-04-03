package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/graph"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/security"
	"github.com/assistclaw/assistclaw/internal/subagents"
	"github.com/assistclaw/assistclaw/internal/system"
)

// SubAgentSvc holds shared dependencies for sub-agent tools (delegation to specialist runners).
type SubAgentSvc struct {
	Store *subagents.Store

	Provider       provider.Provider
	ParentRegistry *agent.ToolRegistry
	ToolGraph      *graph.ToolGraph
	Mem            *memory.Manager
	Log            *zap.Logger

	Model                 string
	ActiveSkillsContext   string
	ProviderName          string
	GatewayPublicBaseURL  string
	ExtensionPromptAppend string
	DefaultToolsProfile   string // security profile for new sub-agents when not specified

	Guardrail *security.Guardrail
	AuditLog  *security.AuditLog
	Hardware  *system.HardwareReport
}

var subAgentOmit = []string{"subagent_create", "subagent_list", "subagent_run", "subagent_remove"}

// buildChildRunner constructs a runner for one sub-agent invocation: same tool stack as the parent
// minus recursive sub-agent tools, isolated workspace, no planning/reflection.
func (s *SubAgentSvc) buildChildRunner(maxIterations int, toolsProfile, workspace string) *agent.Runner {
	childReg := s.ParentRegistry.CloneWithout(subAgentOmit...)
	catalog := NewCatalog(childReg, s.ToolGraph)
	prof := toolsProfile
	if prof != "coding" {
		prof = "full"
	}
	run := agent.NewRunner(agent.Config{
		MaxIterations:         maxIterations,
		Model:                 s.Model,
		ActiveSkillsContext:   s.ActiveSkillsContext,
		ProviderName:          s.ProviderName,
		ToolsProfile:          prof,
		GatewayPublicBaseURL:  s.GatewayPublicBaseURL,
		ExtensionPromptAppend: s.ExtensionPromptAppend,
		EnablePlanning:        false,
		EnableReflection:      false,
		StateDir:              s.Store.StateDir(),
	}, s.Provider, childReg, s.Mem, s.Log, workspace)
	return run.WithCatalog(catalog).WithHardware(s.Hardware).WithSecurity(s.Guardrail, s.AuditLog)
}

// ── subagent_create ──────────────────────────────────────────────────────────

type SubAgentCreateTool struct{ S *SubAgentSvc }

func (t SubAgentCreateTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "subagent_create",
		Description: `Create a named specialist sub-agent with its own workspace under subagents/<id>/ (SOUL.md + instructions). ` +
			`Use for delegating complex parallel concerns (research, IT monitor, writing). The main agent keeps orchestration; ` +
			`invoke work with subagent_run.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Short display name, e.g. NetworkMonitor",
				},
				"instructions": map[string]any{
					"type":        "string",
					"description": "Role, constraints, and expertise for this sub-agent (written to SOUL.md)",
				},
				"tools_profile": map[string]any{
					"type":        "string",
					"description": "Optional: 'full' (default) or 'coding' (no web/browser/cron/message tools)",
					"enum":        []string{"full", "coding"},
				},
			},
			Required: []string{"name", "instructions"},
		},
	}
}

func (t SubAgentCreateTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
		ToolsProfile string `json:"tools_profile"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	prof := args.ToolsProfile
	if prof == "" {
		prof = t.S.DefaultToolsProfile
		if prof == "" {
			prof = "full"
		}
	}
	a, err := t.S.Store.Create(args.Name, args.Instructions, prof)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Created sub-agent %q id=%s tools_profile=%s workspace=%s\nUse subagent_run with id=%s to assign tasks.",
		a.Name, a.ID, a.ToolsProfile, t.S.Store.WorkspaceDir(a.ID), a.ID), nil
}

// ── subagent_list ───────────────────────────────────────────────────────────

type SubAgentListTool struct{ S *SubAgentSvc }

func (t SubAgentListTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "subagent_list",
		Description: "List registered sub-agents (id, name, tools_profile, created_at).",
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: map[string]any{},
		},
	}
}

func (t SubAgentListTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	agents, err := t.S.Store.Load()
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "No sub-agents registered. Use subagent_create first.", nil
	}
	var b strings.Builder
	for _, a := range agents {
		b.WriteString(fmt.Sprintf("- id=%s name=%q profile=%s created=%s\n",
			a.ID, a.Name, a.ToolsProfile, a.CreatedAt.Format(time.RFC3339)))
	}
	return b.String(), nil
}

// ── subagent_run ─────────────────────────────────────────────────────────────

type SubAgentRunTool struct{ S *SubAgentSvc }

func (t SubAgentRunTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "subagent_run",
		Description: `Run a one-shot task with a sub-agent: isolated session, same tools as main (except sub-agent recursion). ` +
			`Returns the sub-agent final text response. Use for heavy research, scripted checks, or drafts.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Sub-agent id from subagent_list / subagent_create",
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Clear task prompt for this invocation",
				},
				"max_iterations": map[string]any{
					"type":        "integer",
					"description": "Optional cap (default 32, max 64)",
				},
			},
			Required: []string{"id", "task"},
		},
	}
}

func (t SubAgentRunTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ID            string `json:"id"`
		Task          string `json:"task"`
		MaxIterations int    `json:"max_iterations"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	sa, err := t.S.Store.Get(args.ID)
	if err != nil {
		return "", err
	}
	if sa == nil {
		return "", fmt.Errorf("unknown sub-agent id %q (use subagent_list)", args.ID)
	}
	maxIt := args.MaxIterations
	if maxIt <= 0 {
		maxIt = 32
	}
	if maxIt > 64 {
		maxIt = 64
	}
	ws := t.S.Store.WorkspaceDir(sa.ID)
	run := t.S.buildChildRunner(maxIt, sa.ToolsProfile, ws)

	sid := "subagent:" + sa.ID + ":" + uuid.New().String()
	runner := run.WithSession(sid)

	runCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	res, err := runner.Run(runCtx, memory.Message{
		ID:        uuid.New().String(),
		SessionID: sid,
		Role:      memory.RoleUser,
		Content:   strings.TrimSpace(args.Task),
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[sub-agent %s %q]\n%s\n---\n(iterations=%d)",
		sa.ID, sa.Name, res.Response, res.Iterations), nil
}

// ── subagent_remove ──────────────────────────────────────────────────────────

type SubAgentRemoveTool struct{ S *SubAgentSvc }

func (t SubAgentRemoveTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "subagent_remove",
		Description: `Remove a sub-agent from the registry. By default deletes its workspace files (SOUL.md, etc.). ` +
			`Use when a specialist is no longer needed.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Sub-agent id",
				},
				"keep_files": map[string]any{
					"type":        "boolean",
					"description": "If true, unregister only but leave workspace on disk (default false = delete files)",
				},
			},
			Required: []string{"id"},
		},
	}
}

func (t SubAgentRemoveTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ID        string `json:"id"`
		KeepFiles bool   `json:"keep_files"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	deleteFiles := !args.KeepFiles
	if err := t.S.Store.Remove(args.ID, deleteFiles); err != nil {
		return "", err
	}
	if deleteFiles {
		return fmt.Sprintf("Removed sub-agent %q and deleted its workspace.", args.ID), nil
	}
	return fmt.Sprintf("Removed sub-agent %q from registry (files kept on disk).", args.ID), nil
}
