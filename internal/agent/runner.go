// Package agent implements the AssistClaw agent runner — the main loop that
// routes messages to LLMs, dispatches tool calls, manages context, and
// writes to all three memory tiers.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// ─────────────────────────────────────────────
// Tool interface
// ─────────────────────────────────────────────

// Tool is the interface that all built-in and user-generated tools must implement.
type Tool interface {
	// Definition returns the schema passed to the LLM.
	Definition() provider.ToolDef
	// Execute runs the tool with the given JSON input.
	// Returns (output string, error). Non-fatal errors should be returned
	// as output strings so the LLM can reason about them.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// ToolRegistry maps tool names to implementations.
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool.
func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Definition().Name] = t
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns all tool definitions for LLM requests.
func (r *ToolRegistry) Definitions() []provider.ToolDef {
	defs := make([]provider.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// ─────────────────────────────────────────────
// Runner
// ─────────────────────────────────────────────

// Config holds runner-specific settings.
type Config struct {
	MaxIterations       int
	SystemPrompt        string
	Model               string
	ActiveSkillsContext string
}

// Runner is the main agent execution loop.
type Runner struct {
	cfg      Config
	provider provider.Provider
	tools    *ToolRegistry
	memory   *memory.Manager
	log      *zap.Logger

	sessionID string
}

// NewRunner creates a new agent runner.
func NewRunner(
	cfg Config,
	p provider.Provider,
	tools *ToolRegistry,
	mem *memory.Manager,
	log *zap.Logger,
) *Runner {
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 64
	}
	return &Runner{
		cfg:       cfg,
		provider:  p,
		tools:     tools,
		memory:    mem,
		log:       log,
		sessionID: uuid.New().String(),
	}
}

// SessionID returns the current session ID.
func (r *Runner) SessionID() string { return r.sessionID }

// RunResult holds the outcome of a Run call.
type RunResult struct {
	SessionID  string
	Response   string
	Iterations int
	Usage      provider.TokenUsage
}

// Run processes a user message and returns the assistant's final response.
// It handles the complete tool-use loop: LLM → tool calls → tool results → LLM.
func (r *Runner) Run(ctx context.Context, userMessage string) (*RunResult, error) {
	// Append user message to working memory.
	userMsg := memory.Message{
		ID:        uuid.New().String(),
		SessionID: r.sessionID,
		Role:      memory.RoleUser,
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	r.memory.Working.Append(userMsg)
	if err := r.memory.Episodic.Save(ctx, userMsg); err != nil {
		r.log.Warn("episodic save failed", zap.Error(err))
	}

	var totalUsage provider.TokenUsage
	iterations := 0

	for iterations < r.cfg.MaxIterations {
		iterations++

		// Build the completion request from working memory.
		req := r.buildRequest()

		r.log.Debug("running completion",
			zap.String("model", r.cfg.Model),
			zap.Int("messages", len(req.Messages)),
			zap.Int("iteration", iterations),
		)

		// Stream the response.
		stream, err := r.provider.Stream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: stream: %w", err)
		}

		resp, err := provider.CollectStream(ctx, stream)
		if err != nil {
			return nil, fmt.Errorf("agent: collect stream: %w", err)
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		// Append assistant message to working memory.
		assistantContent := resp.Text()
		toolCalls := resp.ToolCalls()

		assistantMsg := memory.Message{
			ID:        uuid.New().String(),
			SessionID: r.sessionID,
			Role:      memory.RoleAssistant,
			Content:   assistantContent,
			Model:     r.cfg.Model,
			Tokens:    resp.Usage.CompletionTokens,
			CreatedAt: time.Now(),
		}
		r.memory.Working.Append(assistantMsg)
		if err := r.memory.Episodic.Save(ctx, assistantMsg); err != nil {
			r.log.Warn("episodic save failed", zap.Error(err))
		}

		// If no tool calls, we're done.
		if len(toolCalls) == 0 || resp.FinishReason == provider.FinishReasonStop {
			// Compact working memory if over budget.
			r.memory.Working.Compact(r.memory.Working.TotalTokens())

			return &RunResult{
				SessionID:  r.sessionID,
				Response:   assistantContent,
				Iterations: iterations,
				Usage:      totalUsage,
			}, nil
		}

		// Execute tool calls and collect results.
		for _, tc := range toolCalls {
			result := r.executeTool(ctx, tc)

			toolResultMsg := memory.Message{
				ID:        uuid.New().String(),
				SessionID: r.sessionID,
				Role:      memory.RoleTool,
				Content:   result,
				CreatedAt: time.Now(),
			}
			r.memory.Working.Append(toolResultMsg)
			if err := r.memory.Episodic.Save(ctx, toolResultMsg); err != nil {
				r.log.Warn("episodic save failed", zap.Error(err))
			}
		}
	}

	return nil, fmt.Errorf("agent: exceeded max iterations (%d)", r.cfg.MaxIterations)
}

// executeTool runs a single tool call and returns the result string.
func (r *Runner) executeTool(ctx context.Context, tc provider.ContentPart) string {
	tool, ok := r.tools.Get(tc.ToolName)
	if !ok {
		r.log.Warn("tool not found", zap.String("tool", tc.ToolName))
		return fmt.Sprintf("Error: tool %q not found", tc.ToolName)
	}

	inputJSON, err := json.Marshal(tc.ToolInput)
	if err != nil {
		return fmt.Sprintf("Error marshalling tool input: %v", err)
	}

	r.log.Info("tool call",
		zap.String("tool", tc.ToolName),
		zap.String("input", truncate(string(inputJSON), 200)),
	)

	result, err := tool.Execute(ctx, inputJSON)
	if err != nil {
		r.log.Error("tool execution failed",
			zap.String("tool", tc.ToolName),
			zap.Error(err),
		)
		return fmt.Sprintf("Error: %v", err)
	}

	r.log.Info("tool result",
		zap.String("tool", tc.ToolName),
		zap.String("result", truncate(result, 200)),
	)
	return result
}

// buildRequest converts working memory messages to a provider request.
func (r *Runner) buildRequest() *provider.CompletionRequest {
	msgs := r.memory.Working.Messages()
	providerMsgs := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		role := provider.Role(m.Role)
		providerMsgs = append(providerMsgs, provider.Message{
			Role: role,
			Content: []provider.ContentPart{
				{Type: provider.ContentTypeText, Text: m.Content},
			},
		})
	}

	return &provider.CompletionRequest{
		Model:        r.cfg.Model,
		Messages:     providerMsgs,
		SystemPrompt: r.buildSystemPrompt(),
		Tools:        r.tools.Definitions(),
		MaxTokens:    8096,
		Stream:       true,
	}
}

func (r *Runner) buildSystemPrompt() string {
	base := "You are AssistClaw, a powerful AI assistant with hardware integration and autonomous tool generation capabilities."

	var parts []string
	parts = append(parts, base)

	if r.cfg.SystemPrompt != "" {
		parts = append(parts, r.cfg.SystemPrompt)
	}
	if r.cfg.ActiveSkillsContext != "" {
		parts = append(parts, r.cfg.ActiveSkillsContext)
	}

	return strings.Join(parts, "\n\n")
}

// ─────────────────────────────────────────────
// Stream runner (for interactive CLI use)
// ─────────────────────────────────────────────

// StreamHandler receives streaming events for real-time display.
type StreamHandler interface {
	OnToken(token string)
	OnToolCall(name string, input json.RawMessage)
	OnToolResult(name string, result string)
	OnDone(result *RunResult)
	OnError(err error)
}

// RunStream runs the agent loop and calls handler methods as events occur.
// Designed for live terminal interaction.
func (r *Runner) RunStream(ctx context.Context, userMessage string, handler StreamHandler) {
	// Append user message.
	userMsg := memory.Message{
		ID:        uuid.New().String(),
		SessionID: r.sessionID,
		Role:      memory.RoleUser,
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	r.memory.Working.Append(userMsg)
	_ = r.memory.Episodic.Save(ctx, userMsg)

	var totalUsage provider.TokenUsage
	var fullResponse strings.Builder
	iterations := 0

	for iterations < r.cfg.MaxIterations {
		iterations++
		fullResponse.Reset()

		stream, err := r.provider.Stream(ctx, r.buildRequest())
		if err != nil {
			handler.OnError(fmt.Errorf("agent: stream: %w", err))
			return
		}

		var toolCalls []provider.ContentPart
		var finishReason provider.FinishReason

		for event := range stream {
			switch event.Type {
			case provider.StreamEventText:
				handler.OnToken(event.Text)
				fullResponse.WriteString(event.Text)
			case provider.StreamEventToolUse:
				if event.ToolUse != nil {
					toolCalls = append(toolCalls, *event.ToolUse)
				}
			case provider.StreamEventDone:
				finishReason = event.FinishReason
				if event.Usage != nil {
					totalUsage.PromptTokens += event.Usage.PromptTokens
					totalUsage.CompletionTokens += event.Usage.CompletionTokens
					totalUsage.TotalTokens += event.Usage.TotalTokens
				}
			case provider.StreamEventError:
				handler.OnError(event.Err)
				return
			}
		}

		assistantMsg := memory.Message{
			ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleAssistant,
			Content: fullResponse.String(), Model: r.cfg.Model,
			Tokens: totalUsage.CompletionTokens, CreatedAt: time.Now(),
		}
		r.memory.Working.Append(assistantMsg)
		_ = r.memory.Episodic.Save(ctx, assistantMsg)

		if len(toolCalls) == 0 || finishReason == provider.FinishReasonStop {
			handler.OnDone(&RunResult{
				SessionID: r.sessionID, Response: fullResponse.String(),
				Iterations: iterations, Usage: totalUsage,
			})
			return
		}

		for _, tc := range toolCalls {
			inputJSON, _ := json.Marshal(tc.ToolInput)
			handler.OnToolCall(tc.ToolName, inputJSON)
			result := r.executeTool(ctx, tc)
			handler.OnToolResult(tc.ToolName, result)

			toolMsg := memory.Message{
				ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleTool,
				Content: result, CreatedAt: time.Now(),
			}
			r.memory.Working.Append(toolMsg)
			_ = r.memory.Episodic.Save(ctx, toolMsg)
		}
	}

	handler.OnError(fmt.Errorf("agent: exceeded max iterations (%d)", r.cfg.MaxIterations))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
