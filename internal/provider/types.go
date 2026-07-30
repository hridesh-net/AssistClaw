// Package provider defines the unified LLM provider interface and shared types
// for AssistClaw's model-agnostic orchestration layer.
//
// The contract types (Provider, CompletionRequest/Response, StreamEvent,
// ModelInfo, ProviderCaps, ...) now live in internal/core — the std-only
// contract layer. This file re-exports them as type aliases so existing
// references (provider.Provider, provider.ToolDef, ...) continue to compile
// unchanged. New code should import internal/core directly.
package provider

import (
	"github.com/assistclaw/assistclaw/internal/core"
)

// ─────────────────────────────────────────────
// Core types (aliases to internal/core)
// ─────────────────────────────────────────────

type (
	Role               = core.Role
	ContentType        = core.ContentType
	ContentPart        = core.ContentPart
	Message            = core.Message
	ToolParameter      = core.ToolParameter
	ToolDef            = core.ToolDef
	CompletionRequest  = core.CompletionRequest
	TokenUsage         = core.TokenUsage
	FinishReason       = core.FinishReason
	CompletionResponse = core.CompletionResponse
	StreamEventType    = core.StreamEventType
	StreamEvent        = core.StreamEvent
	Capability         = core.Capability
	ModelInfo          = core.ModelInfo
	Provider           = core.Provider
	StreamingProvider  = core.StreamingProvider
	ProviderError      = core.ProviderError
)

const (
	RoleSystem    = core.RoleSystem
	RoleUser      = core.RoleUser
	RoleAssistant = core.RoleAssistant
	RoleTool      = core.RoleTool

	ContentTypeText       = core.ContentTypeText
	ContentTypeImage      = core.ContentTypeImage
	ContentTypeAudio      = core.ContentTypeAudio
	ContentTypeToolUse    = core.ContentTypeToolUse
	ContentTypeToolResult = core.ContentTypeToolResult

	FinishReasonStop     = core.FinishReasonStop
	FinishReasonLength   = core.FinishReasonLength
	FinishReasonToolUse  = core.FinishReasonToolUse
	FinishReasonFiltered = core.FinishReasonFiltered

	StreamEventText    = core.StreamEventText
	StreamEventToolUse = core.StreamEventToolUse
	StreamEventDone    = core.StreamEventDone
	StreamEventError   = core.StreamEventError

	CapabilityVision    = core.CapabilityVision
	CapabilityTools     = core.CapabilityTools
	CapabilityReasoning = core.CapabilityReasoning
	CapabilityStreaming = core.CapabilityStreaming
	CapabilityJSON      = core.CapabilityJSON
	CapabilityEmbedding = core.CapabilityEmbedding
)

// Re-exported functions (see internal/core for implementations).
var (
	NewTextMessage = core.NewTextMessage
	DrainStream    = core.DrainStream
	CollectStream  = core.CollectStream
	StreamToWriter = core.StreamToWriter
)
