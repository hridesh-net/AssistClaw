package tui

import (
	"context"
	"encoding/json"
)

// AgentStreamHandler mirrors agent.StreamHandler so the TUI package does not
// depend on the agent package directly. main.go adapts between the two.
type AgentStreamHandler interface {
	OnToken(token string)
	OnToolCall(name string, input json.RawMessage)
	OnToolResult(name, result string)
	OnDone(result *RunResult)
	OnError(err error)
}

// ─────────────────────────────────────────────
// RunREPL — entry point called from main.go
// ─────────────────────────────────────────────

// RunREPL starts the Rust TUI REPL.
// It bridges agent streaming events into the TUI via the C FFI layer.
// `noMouse=true` forces mouse capture off (recommended inside tmux/screen).
func RunREPL(ctx context.Context, runner AgentRunner, ver string, providerCount, skillCount int, noMouse bool) error {
	bridge := NewBridge(runner, ver, providerCount, skillCount)
	if noMouse {
		bridge.MouseOverride = "off"
	}
	return bridge.StartREPL(ctx)
}
