package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AgentRunner is the minimal surface area of agent.Runner needed by the TUI.
// RunStream blocks until the run completes, reporting progress via the handler.
type AgentRunner interface {
	SessionID() string
	RunStream(ctx context.Context, userMessage string, h AgentStreamHandler)
}

// RunResult mirrors agent.RunResult so the TUI package does not depend on the
// agent package directly.
type RunResult struct {
	Iterations int
	Usage      struct{ TotalTokens int }
}

// Bridge connects the Go agent runner with the Rust TUI via the C FFI layer.
//
// One bridge instance owns:
//   - the event-polling goroutine (Rust → Go events at ~60 Hz)
//   - the event-dispatch goroutine (UserMessage → agent.Run)
//   - the lifecycle (clean shutdown on Quit/Interrupt or ctx cancel)
type Bridge struct {
	runner        AgentRunner
	version       string
	providerCount int
	skillCount    int

	// MouseOverride controls Rust-side mouse capture: "auto" (default —
	// Rust detects tmux/screen), "on", or "off". Callers like main.go
	// translate the --no-mouse flag into "off".
	MouseOverride string

	eventChan chan tuiEvent

	// runCancel cancels an in-flight agent run when the user hits Ctrl+C.
	mu        sync.Mutex
	runCancel context.CancelFunc

	// shutdownOnce guards close(shutdown) so it is safe to call from
	// multiple goroutines.
	shutdownOnce sync.Once
	shutdown     chan struct{}
}

// tuiEvent mirrors the Rust event types for JSON deserialisation.
// Payload is only populated for variants whose serde representation is a
// bare string (UserMessage, Navigate, OnboardComplete, SkillsComplete).
type tuiEvent struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// NewBridge creates a new bridge for the given agent runner.
func NewBridge(runner AgentRunner, version string, providerCount, skillCount int) *Bridge {
	return &Bridge{
		runner:        runner,
		version:       version,
		providerCount: providerCount,
		skillCount:    skillCount,
		eventChan:     make(chan tuiEvent, 64),
		shutdown:      make(chan struct{}),
	}
}

// StartREPL boots the Rust TUI and runs the event loop until the user quits
// (Esc, Ctrl+C, or window close) or the context is cancelled.
func (b *Bridge) StartREPL(ctx context.Context) error {
	if err := Init(); err != nil {
		return fmt.Errorf("init tui: %w", err)
	}
	defer Shutdown()

	configMap := map[string]any{
		"version":        b.version,
		"session_id":     b.runner.SessionID(),
		"provider_count": b.providerCount,
		"skill_count":    b.skillCount,
	}
	switch b.MouseOverride {
	case "on":
		configMap["enable_mouse"] = true
	case "off":
		configMap["enable_mouse"] = false
	}
	config, _ := json.Marshal(configMap)

	if err := ReplStart(string(config)); err != nil {
		return fmt.Errorf("start repl: %w", err)
	}
	defer ReplStop()

	// Derive a child context so we can cancel both goroutines on user quit.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go b.pollEvents(runCtx)
	go b.handleEvents(runCtx)

	select {
	case <-ctx.Done():
	case <-b.shutdown:
	}
	return nil
}

// signalShutdown is the idempotent way to ask StartREPL to return.
func (b *Bridge) signalShutdown() {
	b.shutdownOnce.Do(func() { close(b.shutdown) })
}

// pollEvents reads events from the Rust TUI at ~60 Hz and forwards them
// into the event channel for dispatch. Returns when ctx is cancelled or
// shutdown has been signalled.
func (b *Bridge) pollEvents(ctx context.Context) {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.shutdown:
			return
		case <-ticker.C:
			eventJSON := PollEvent()
			if eventJSON == "" {
				continue
			}

			var evt tuiEvent
			// Resize / Action events carry object payloads and won't fit
			// this struct — that's fine, we don't dispatch them yet.
			if err := json.Unmarshal([]byte(eventJSON), &evt); err != nil {
				continue
			}

			select {
			case b.eventChan <- evt:
			case <-ctx.Done():
				return
			case <-b.shutdown:
				return
			}
		}
	}
}

// handleEvents dispatches Rust → Go events to the agent runner and the
// shutdown channel as appropriate.
func (b *Bridge) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.shutdown:
			return
		case evt := <-b.eventChan:
			switch evt.Type {
			case "UserMessage":
				go b.runAgent(ctx, evt.Payload)
			case "Interrupt":
				// Cancel the in-flight run if any. Do NOT shut down the
				// REPL — the user just wants to stop the current request.
				b.mu.Lock()
				if b.runCancel != nil {
					b.runCancel()
					b.runCancel = nil
				}
				b.mu.Unlock()
			case "Quit":
				b.signalShutdown()
				return
			}
		}
	}
}

// runAgent executes the agent loop and streams the result back to the Rust TUI.
// Holds a cancellable child context so a follow-up Interrupt event can stop
// the request in flight.
func (b *Bridge) runAgent(parent context.Context, message string) {
	ctx, cancel := context.WithCancel(parent)

	b.mu.Lock()
	if b.runCancel != nil {
		// Cancel any previous run before we replace it.
		b.runCancel()
	}
	b.runCancel = cancel
	b.mu.Unlock()

	defer func() {
		cancel()
		b.mu.Lock()
		if b.runCancel != nil && ctx.Err() != nil {
			b.runCancel = nil
		}
		b.mu.Unlock()
	}()

	// Empty token = run started: clears any stale streaming buffer and turns
	// the thinking indicator on in the Rust REPL.
	ReplSendToken("")

	b.runner.RunStream(ctx, message, ffiStreamHandler{})
}

// ffiStreamHandler forwards agent streaming events into the Rust TUI.
type ffiStreamHandler struct{}

func (ffiStreamHandler) OnToken(token string) {
	if token != "" { // empty string is the clear/run-start sentinel
		ReplSendToken(token)
	}
}

func (ffiStreamHandler) OnToolCall(name string, _ json.RawMessage) {
	ReplSendTool(name, "")
}

func (ffiStreamHandler) OnToolResult(string, string) {
	ReplSendTool("", "") // clear the tool indicator
}

func (ffiStreamHandler) OnDone(result *RunResult) {
	if result == nil {
		result = &RunResult{}
	}
	ReplSendDone(result.Iterations, result.Usage.TotalTokens)
}

func (ffiStreamHandler) OnError(err error) {
	if errors.Is(err, context.Canceled) {
		ReplSendError("interrupted")
		return
	}
	ReplSendError(err.Error())
}
