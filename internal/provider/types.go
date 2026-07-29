// Package provider defines the unified LLM provider interface and shared types
// for AssistClaw's model-agnostic orchestration layer.
package provider

import (
	"context"
	"io"
	"strings"
	"time"
)

// ─────────────────────────────────────────────
// Core types
// ─────────────────────────────────────────────

// Role represents the conversation participant role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType describes the modality of a content part.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeAudio      ContentType = "audio"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentPart is a single piece of content within a message.
type ContentPart struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL string      `json:"image_url,omitempty"`
	// For image data embedded directly
	ImageData     []byte `json:"image_data,omitempty"`
	ImageMimeType string `json:"image_mime_type,omitempty"`
	// For tool use
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput any    `json:"tool_input,omitempty"`
	// For tool results
	ToolResultID      string `json:"tool_result_id,omitempty"`
	ToolResultContent string `json:"tool_result_content,omitempty"`
	ToolResultError   bool   `json:"tool_result_error,omitempty"`
}

// Message represents a single turn in the conversation.
type Message struct {
	Role    Role          `json:"role"`
	Content []ContentPart `json:"content"`
	// Name is used for tool result messages (the tool's function name).
	Name string `json:"name,omitempty"`
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role:    role,
		Content: []ContentPart{{Type: ContentTypeText, Text: text}},
	}
}

// ─────────────────────────────────────────────
// Tool definitions
// ─────────────────────────────────────────────

// ToolParameter defines a single parameter for a tool.
type ToolParameter struct {
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Enum        []string       `json:"enum,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Required    []string       `json:"required,omitempty"`
	Items       *ToolParameter `json:"items,omitempty"`
}

// ToolDef defines a tool that a model can call.
type ToolDef struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	InputSchema ToolParameter `json:"input_schema"`
}

// ─────────────────────────────────────────────
// Request / Response
// ─────────────────────────────────────────────

// CompletionRequest is the unified request sent to any LLM provider.
type CompletionRequest struct {
	Model        string    `json:"model"`
	Messages     []Message `json:"messages"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Tools        []ToolDef `json:"tools,omitempty"`
	// Generation parameters
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	// Streaming
	Stream bool `json:"stream,omitempty"`
	// Provider-specific extensions (passed through as-is)
	Extra map[string]any `json:"extra,omitempty"`
}

// TokenUsage reports token consumption.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens"`
}

// FinishReason indicates why generation stopped.
type FinishReason string

const (
	FinishReasonStop     FinishReason = "stop"
	FinishReasonLength   FinishReason = "length"
	FinishReasonToolUse  FinishReason = "tool_use"
	FinishReasonFiltered FinishReason = "content_filter"
)

// CompletionResponse is the unified response from any LLM provider.
type CompletionResponse struct {
	ID           string        `json:"id"`
	Model        string        `json:"model"`
	Content      []ContentPart `json:"content"`
	FinishReason FinishReason  `json:"finish_reason"`
	Usage        TokenUsage    `json:"usage"`
	Latency      time.Duration `json:"latency_ms"`
}

// Text returns the concatenated text content from the response.
func (r *CompletionResponse) Text() string {
	var b strings.Builder
	for _, part := range r.Content {
		if part.Type == ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// ToolCalls returns all tool use content parts from the response.
func (r *CompletionResponse) ToolCalls() []ContentPart {
	var calls []ContentPart
	for _, part := range r.Content {
		if part.Type == ContentTypeToolUse {
			calls = append(calls, part)
		}
	}
	return calls
}

// ─────────────────────────────────────────────
// Streaming
// ─────────────────────────────────────────────

// StreamEventType classifies a streaming event.
type StreamEventType string

const (
	StreamEventText    StreamEventType = "text"
	StreamEventToolUse StreamEventType = "tool_use"
	StreamEventDone    StreamEventType = "done"
	StreamEventError   StreamEventType = "error"
)

// StreamEvent is a single event from a streaming response.
type StreamEvent struct {
	Type         StreamEventType `json:"type"`
	Text         string          `json:"text,omitempty"`
	ToolUse      *ContentPart    `json:"tool_use,omitempty"`
	Usage        *TokenUsage     `json:"usage,omitempty"`
	FinishReason FinishReason    `json:"finish_reason,omitempty"`
	Err          error           `json:"-"`
}

// ─────────────────────────────────────────────
// Model info
// ─────────────────────────────────────────────

// Capability flags for model features.
type Capability string

const (
	CapabilityVision    Capability = "vision"
	CapabilityTools     Capability = "tools"
	CapabilityReasoning Capability = "reasoning"
	CapabilityStreaming Capability = "streaming"
	CapabilityJSON      Capability = "json_mode"
	CapabilityEmbedding Capability = "embedding"
)

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Provider     string       `json:"provider"`
	Capabilities []Capability `json:"capabilities"`
	// Pricing per million tokens (in USD)
	InputCostPerM  float64 `json:"input_cost_per_m,omitempty"`
	OutputCostPerM float64 `json:"output_cost_per_m,omitempty"`
	// Limits
	ContextWindow   int `json:"context_window"`
	MaxOutputTokens int `json:"max_output_tokens"`
	// Local indicates the model runs on the local machine.
	Local bool `json:"local"`
}

// HasCapability checks if the model supports a given capability.
func (m *ModelInfo) HasCapability(c Capability) bool {
	for _, cap := range m.Capabilities {
		if cap == c {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────
// Provider interface
// ─────────────────────────────────────────────

// Provider is the unified interface that every LLM backend must implement.
// Each provider package returns an implementation of this interface.
type Provider interface {
	// Name returns the canonical provider identifier (e.g. "openai", "anthropic").
	Name() string

	// Complete performs a blocking completion.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// Stream initiates a streaming completion. The caller must fully consume
	// (or drain) the returned channel, which is closed when the stream ends.
	Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)

	// ListModels returns all models available from this provider.
	// May perform a network request for dynamic providers (Ollama, vLLM).
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// HealthCheck verifies the provider is reachable and credentials are valid.
	HealthCheck(ctx context.Context) error

	// Embed generates a vector representation of the given text.
	Embed(ctx context.Context, model string, text string) ([]float32, error)

	// ValidateModel verifies if a model ID is valid for this provider.
	ValidateModel(ctx context.Context, modelID string) error

	// Caps returns tool-calling constraints for this backend (used for catalog sizing).
	Caps() ProviderCaps
}

// StreamingProvider optionally indicates the provider supports streaming in
// addition to blocking calls. All providers must implement Stream(), but this
// interface is used internally for capability assertion.
type StreamingProvider interface {
	Provider
	SupportsNativeStreaming() bool
}

// ─────────────────────────────────────────────
// Common errors
// ─────────────────────────────────────────────

// ProviderError wraps errors with provider context.
type ProviderError struct {
	Provider   string
	StatusCode int
	Message    string
	Retryable  bool
	Err        error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Provider + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Provider + ": " + e.Message
}

func (e *ProviderError) Unwrap() error { return e.Err }

// DrainStream reads and discards all events from a stream channel until closed.
// Used in deferred calls to prevent goroutine leaks.
func DrainStream(ch <-chan StreamEvent) {
	for range ch {
	}
}

// CollectStream reads all events from a stream and assembles a CompletionResponse.
func CollectStream(ctx context.Context, ch <-chan StreamEvent) (*CompletionResponse, error) {
	var b strings.Builder
	var toolCalls []ContentPart
	var usage *TokenUsage
	var finishReason FinishReason

	for event := range ch {
		select {
		case <-ctx.Done():
			// Drain remaining events
			DrainStream(ch)
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case StreamEventText:
			b.WriteString(event.Text)
		case StreamEventToolUse:
			if event.ToolUse != nil {
				toolCalls = append(toolCalls, *event.ToolUse)
			}
		case StreamEventDone:
			if event.Usage != nil {
				usage = event.Usage
			}
			finishReason = event.FinishReason
		case StreamEventError:
			return nil, event.Err
		}
	}

	content := make([]ContentPart, 0, 1+len(toolCalls))
	text := b.String()
	if text != "" {
		content = append(content, ContentPart{Type: ContentTypeText, Text: text})
	}
	content = append(content, toolCalls...)

	resp := &CompletionResponse{
		Content:      content,
		FinishReason: finishReason,
	}
	if usage != nil {
		resp.Usage = *usage
	}
	return resp, nil
}

// StreamToWriter streams text events to the provided writer.
// Returns the full CompletionResponse once streaming is done.
func StreamToWriter(ctx context.Context, ch <-chan StreamEvent, w io.Writer) (*CompletionResponse, error) {
	var b strings.Builder
	var toolCalls []ContentPart
	var usage *TokenUsage
	var finishReason FinishReason

	for event := range ch {
		select {
		case <-ctx.Done():
			DrainStream(ch)
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case StreamEventText:
			b.WriteString(event.Text)
			if _, err := io.WriteString(w, event.Text); err != nil {
				go DrainStream(ch)
				return nil, err
			}
		case StreamEventToolUse:
			if event.ToolUse != nil {
				toolCalls = append(toolCalls, *event.ToolUse)
			}
		case StreamEventDone:
			if event.Usage != nil {
				usage = event.Usage
			}
			finishReason = event.FinishReason
		case StreamEventError:
			return nil, event.Err
		}
	}

	content := make([]ContentPart, 0, 1+len(toolCalls))
	text := b.String()
	if text != "" {
		content = append(content, ContentPart{Type: ContentTypeText, Text: text})
	}
	content = append(content, toolCalls...)

	resp := &CompletionResponse{
		Content:      content,
		FinishReason: finishReason,
	}
	if usage != nil {
		resp.Usage = *usage
	}
	return resp, nil
}
