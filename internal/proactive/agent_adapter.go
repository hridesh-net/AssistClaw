package proactive

import (
	"context"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/google/uuid"
)

// RunnerAdapter wraps *agent.Runner to satisfy the AgentInvoker interface.
// It converts the string prompt into a memory.Message and runs the full agent loop.
type RunnerAdapter struct {
	runner *agent.Runner
}

// NewRunnerAdapter creates an adapter around the given agent runner.
func NewRunnerAdapter(runner *agent.Runner) *RunnerAdapter {
	return &RunnerAdapter{runner: runner}
}

// Run invokes the agent with the given prompt and returns the response text.
func (a *RunnerAdapter) Run(ctx context.Context, prompt string) (string, error) {
	msg := memory.Message{
		ID:        uuid.New().String(),
		SessionID: "proactive:" + uuid.New().String()[:8],
		Role:      memory.RoleUser,
		Content:   prompt,
	}
	res, err := a.runner.Run(ctx, msg)
	if err != nil {
		return "", err
	}
	return res.Response, nil
}
