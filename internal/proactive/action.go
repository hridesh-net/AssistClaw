package proactive

import (
	"context"
	"fmt"
)

// AgentInvoker is the point-of-consumption interface for running the agent
// from the proactive engine. It avoids a direct dependency on *agent.Runner.
type AgentInvoker interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// RunAgentAction invokes the LLM agent with the compiled rule prompt.
type RunAgentAction struct {
	invoker AgentInvoker
}

// NewRunAgentAction creates an action that delegates to the given invoker.
func NewRunAgentAction(invoker AgentInvoker) *RunAgentAction {
	return &RunAgentAction{invoker: invoker}
}

// Name returns "run_agent".
func (a *RunAgentAction) Name() string { return "run_agent" }

// Execute runs the agent with the rule's compiled prompt.
func (a *RunAgentAction) Execute(ctx context.Context, ev Event, rule Rule) (string, error) {
	prompt := rule.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf("Proactive event from %s (%s): %+v", ev.Source, ev.Type, ev.Payload)
	}
	return a.invoker.Run(ctx, prompt)
}

// ShellAction executes a shell command (off by default; requires allowlist).
type ShellAction struct {
	allowlist map[string]bool // command basename allowlist
}

// NewShellAction creates a shell action with the given allowlist.
// An empty allowlist means all commands are rejected.
func NewShellAction(allowlist []string) *ShellAction {
	s := &ShellAction{allowlist: make(map[string]bool, len(allowlist))}
	for _, cmd := range allowlist {
		s.allowlist[cmd] = true
	}
	return s
}

// Name returns "shell".
func (a *ShellAction) Name() string { return "shell" }

// Execute runs the allowed shell command. Rejected commands return an error.
func (a *ShellAction) Execute(ctx context.Context, ev Event, rule Rule) (string, error) {
	return "", fmt.Errorf("shell action not yet implemented")
}
