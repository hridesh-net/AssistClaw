package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/localintel"
)

// LocalIntelProvider adapts a localintel.Engine to the provider.Provider interface.
// It is used as the fallback in FailoverProvider when all remote providers fail.
type LocalIntelProvider struct {
	eng        localintel.Engine
	modelID    string
	maxTokens  int
	systemPrompt string
}

// NewLocalIntelProvider creates a provider wrapper around the given local engine.
func NewLocalIntelProvider(eng localintel.Engine, modelID, systemPrompt string, maxTokens int) *LocalIntelProvider {
	return &LocalIntelProvider{
		eng:          eng,
		modelID:      modelID,
		maxTokens:    maxTokens,
		systemPrompt: systemPrompt,
	}
}

// Name returns "localintel".
func (l *LocalIntelProvider) Name() string { return "localintel" }

// Complete delegates to the local engine.
func (l *LocalIntelProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	user := ""
	for _, m := range req.Messages {
		for _, part := range m.Content {
			if part.Type == ContentTypeText {
				user += part.Text + "\n"
			}
		}
	}
	start := time.Now()
	text, err := l.eng.Complete(ctx, localintel.Request{
		System:    l.systemPrompt,
		User:      strings.TrimSpace(user),
		MaxTokens: l.maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("localintel: %w", err)
	}
	return &CompletionResponse{
		Model:        l.modelID,
		Content:      []ContentPart{{Type: ContentTypeText, Text: text}},
		FinishReason: FinishReasonStop,
		Latency:      time.Since(start),
	}, nil
}

// Stream is not supported by local intel; returns an error.
func (l *LocalIntelProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("localintel: streaming not supported")
}

// ListModels returns the local model only.
func (l *LocalIntelProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: l.modelID, Provider: l.Name()}}, nil
}

// HealthCheck verifies the engine is available.
func (l *LocalIntelProvider) HealthCheck(ctx context.Context) error {
	if l.eng == nil || !l.eng.Available() {
		return fmt.Errorf("localintel engine not available")
	}
	return nil
}

// Embed is not supported by local intel.
func (l *LocalIntelProvider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return nil, fmt.Errorf("localintel: embed not supported")
}

// ValidateModel accepts only the configured local model ID.
func (l *LocalIntelProvider) ValidateModel(ctx context.Context, modelID string) error {
	if modelID == l.modelID {
		return nil
	}
	return fmt.Errorf("localintel: unknown model %q", modelID)
}

// Caps returns conservative caps (no tool calling for gemma-2-2b).
func (l *LocalIntelProvider) Caps() ProviderCaps {
	return ProviderCaps{
		MaxTools:           0,
		RequiresAllTools:   false,
		NativeToolUse:      false,
		SupportsToolResult: false,
	}
}
