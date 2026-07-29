package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// This file is the parity harness for the agent turn loop. internal/agent had
// no tests; these golden tests pin the observable behavior of Runner.Run (text
// answers and the tool-call loop) using a scripted fake provider — no network,
// no real model. They must keep passing byte-for-byte through the structural
// refactors (kernel extraction, Runner decomposition in WS2): if the turn loop
// is moved or split, these are the contract it has to honor.

// scriptedTurn is one model response the fake provider will emit, in order.
type scriptedTurn struct {
	text     string         // assistant text to stream (may be empty)
	toolName string         // if set, emit a tool_use for this tool
	toolID   string         // tool_use id
	toolArgs map[string]any // tool input
	finish   provider.FinishReason
}

// fakeProvider implements provider.Provider (core.Provider) and returns a
// pre-scripted sequence of responses, one per Stream call.
type fakeProvider struct {
	turns []scriptedTurn
	calls int // number of Stream invocations so far
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	// Not exercised by Run (which streams); provide a minimal implementation.
	return &provider.CompletionResponse{FinishReason: provider.FinishReasonStop}, nil
}

func (f *fakeProvider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	idx := f.calls
	f.calls++
	ch := make(chan provider.StreamEvent, 4)
	go func() {
		defer close(ch)
		if idx >= len(f.turns) {
			// Ran out of script: emit an empty stop so the loop terminates.
			ch <- provider.StreamEvent{Type: provider.StreamEventDone, FinishReason: provider.FinishReasonStop, Usage: &provider.TokenUsage{}}
			return
		}
		turn := f.turns[idx]
		if turn.text != "" {
			ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: turn.text}
		}
		if turn.toolName != "" {
			ch <- provider.StreamEvent{Type: provider.StreamEventToolUse, ToolUse: &provider.ContentPart{
				Type:      provider.ContentTypeToolUse,
				ToolName:  turn.toolName,
				ToolUseID: turn.toolID,
				ToolInput: turn.toolArgs,
			}}
		}
		finish := turn.finish
		if finish == "" {
			if turn.toolName != "" {
				finish = provider.FinishReasonToolUse
			} else {
				finish = provider.FinishReasonStop
			}
		}
		ch <- provider.StreamEvent{Type: provider.StreamEventDone, FinishReason: finish, Usage: &provider.TokenUsage{TotalTokens: 3}}
	}()
	return ch, nil
}

func (f *fakeProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "fake/model", Provider: "fake"}}, nil
}
func (f *fakeProvider) HealthCheck(ctx context.Context) error { return nil }
func (f *fakeProvider) Embed(ctx context.Context, model, text string) ([]float32, error) {
	return nil, nil
}
func (f *fakeProvider) ValidateModel(ctx context.Context, modelID string) error { return nil }
func (f *fakeProvider) Caps() provider.ProviderCaps {
	return provider.ProviderCaps{NativeToolUse: true, SupportsToolResult: true}
}

// echoTool is a trivial tool that records that it was called and echoes its arg.
type echoTool struct{ called *int }

func (t *echoTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "echo",
		Description: "echoes the msg argument",
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: map[string]any{"msg": map[string]any{"type": "string"}},
			Required:   []string{"msg"},
		},
	}
}

func (t *echoTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	*t.called++
	var in struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(input, &in)
	return "echoed: " + in.Msg, nil
}

func newTestMemory(t *testing.T) *memory.Manager {
	t.Helper()
	dir := t.TempDir()
	mem, err := memory.NewManager(memory.ManagerConfig{
		WorkingTokenBudget:  8000,
		EpisodicDBPath:      filepath.Join(dir, "episodic.db"),
		SemanticDBPath:      filepath.Join(dir, "semantic.db"),
		EmbeddingDimensions: 1536,
	})
	if err != nil {
		t.Fatalf("memory.NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	return mem
}

// TestRunner_TurnLoop_TextResponse: a plain text answer returns in one iteration.
func TestRunner_TurnLoop_TextResponse(t *testing.T) {
	mem := newTestMemory(t)
	fp := &fakeProvider{turns: []scriptedTurn{{text: "hello there"}}}
	r := NewRunner(Config{Model: "fake/model", MaxIterations: 5}, fp, NewToolRegistry(), mem, zap.NewNop(), t.TempDir()).
		WithSession("golden-text")

	res, err := r.Run(context.Background(), memory.Message{
		SessionID: "golden-text", Role: memory.RoleUser, Content: "hi",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Response != "hello there" {
		t.Errorf("Response = %q, want %q", res.Response, "hello there")
	}
	if res.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1", res.Iterations)
	}
	if fp.calls != 1 {
		t.Errorf("provider Stream called %d times, want 1", fp.calls)
	}
}

// TestRunner_TurnLoop_ToolCall: a tool_use response drives one tool execution,
// then a follow-up text response ends the turn. This pins the tool-dispatch
// loop that WS2's Runner decomposition must preserve.
func TestRunner_TurnLoop_ToolCall(t *testing.T) {
	mem := newTestMemory(t)
	called := 0
	reg := NewToolRegistry()
	reg.Register(&echoTool{called: &called})

	fp := &fakeProvider{turns: []scriptedTurn{
		{toolName: "echo", toolID: "call-1", toolArgs: map[string]any{"msg": "world"}},
		{text: "done: world"},
	}}
	r := NewRunner(Config{Model: "fake/model", MaxIterations: 5}, fp, reg, mem, zap.NewNop(), t.TempDir()).
		WithSession("golden-tool")

	res, err := r.Run(context.Background(), memory.Message{
		SessionID: "golden-tool", Role: memory.RoleUser, Content: "use echo",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called != 1 {
		t.Errorf("echo tool called %d times, want 1", called)
	}
	if res.Response != "done: world" {
		t.Errorf("Response = %q, want %q", res.Response, "done: world")
	}
	if res.Iterations != 2 {
		t.Errorf("Iterations = %d, want 2 (tool turn + final text)", res.Iterations)
	}
}
