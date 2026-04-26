// Package anthropic implements the Anthropic Messages API provider for AssistClaw.
// Supports claude-3/4 model family, streaming, tool use, vision, and prompt caching.
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

const (
	defaultBaseURL = "https://api.anthropic.com/v1"
	messagesPath   = "/messages"
	modelsPath     = "/models"
	providerName   = "anthropic"
	defaultVersion = "2023-06-01"
	defaultTimeout = 120 * time.Second
)

// Config holds Anthropic provider settings.
type Config struct {
	APIKey       string `yaml:"api_key" json:"api_key"`
	BaseURL      string `yaml:"base_url" json:"base_url"`
	DefaultModel string `yaml:"default_model" json:"default_model"`
	// BetaFeatures enables Anthropic beta headers (e.g. "prompt-caching-2024-07-31")
	BetaFeatures []string `yaml:"beta_features" json:"beta_features"`
}

// Provider implements provider.Provider for Anthropic.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new Anthropic provider.
func New(cfg Config) *Provider {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(base, "/")
	return &Provider{cfg: cfg, client: &http.Client{Timeout: defaultTimeout}}
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Caps() provider.ProviderCaps { return provider.CapsAnthropic() }

func (p *Provider) HealthCheck(ctx context.Context) error {
	return p.ValidateModel(ctx, p.cfg.DefaultModel)
}

func (p *Provider) ValidateModel(ctx context.Context, modelID string) error {
	if modelID == "" {
		modelID = p.cfg.DefaultModel
	}
	if modelID == "" {
		return nil
	}

	models, _ := p.ListModels(ctx)
	for _, m := range models {
		if m.ID == modelID {
			return nil
		}
	}
	return &provider.ProviderError{
		Provider:   providerName,
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("model %q not found", modelID),
	}
}

// ListModels returns the Anthropic model catalog.
func (p *Provider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return anthropicModelCatalog(p.Name()), nil
}

// Complete performs a blocking Anthropic Messages API call.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "complete", Err: err, Retryable: true}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, p.parseError(httpResp)
	}

	var response anthropicResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.toProviderResponse(time.Since(start)), nil
}

// Stream initiates a streaming Anthropic Messages API call.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "stream", Err: err, Retryable: true}
	}
	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, p.parseError(httpResp)
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		p.readSSE(ctx, httpResp, ch)
	}()
	return ch, nil
}

func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return nil, fmt.Errorf("anthropic: embeddings not supported through this provider")
}

func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Request building
// ─────────────────────────────────────────────

type anthropicContent struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Source *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source,omitempty"`
	// Tool use
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
	// Tool result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

func (p *Provider) buildRequestBody(req *provider.CompletionRequest, stream bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8096
	}

	var messages []anthropicMessage
	for _, m := range req.Messages {
		am, err := convertMessage(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, am)
	}

	body := anthropicRequest{
		Model:     model,
		Messages:  messages,
		System:    req.SystemPrompt,
		MaxTokens: maxTokens,
		Stream:    stream,
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return json.Marshal(body)
}

func convertMessage(m provider.Message) (anthropicMessage, error) {
	am := anthropicMessage{Role: string(m.Role)}
	for _, cp := range m.Content {
		var ac anthropicContent
		switch cp.Type {
		case provider.ContentTypeText:
			ac = anthropicContent{Type: "text", Text: cp.Text}
		case provider.ContentTypeImage:
			// Base64-encoded image
			ac = anthropicContent{
				Type: "image",
				Source: &struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				}{
					Type:      "base64",
					MediaType: cp.ImageMimeType,
					Data:      string(cp.ImageData),
				},
			}
		case provider.ContentTypeToolUse:
			ac = anthropicContent{
				Type:  "tool_use",
				ID:    cp.ToolUseID,
				Name:  cp.ToolName,
				Input: cp.ToolInput,
			}
		case provider.ContentTypeToolResult:
			ac = anthropicContent{
				Type:      "tool_result",
				ToolUseID: cp.ToolResultID,
				Content:   cp.ToolResultContent,
				IsError:   cp.ToolResultError,
			}
		}
		am.Content = append(am.Content, ac)
	}
	return am, nil
}

// ─────────────────────────────────────────────
// HTTP request
// ─────────────────────────────────────────────

func (p *Provider) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	url := p.cfg.BaseURL + messagesPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", defaultVersion)
	if len(p.cfg.BetaFeatures) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(p.cfg.BetaFeatures, ","))
	}
	return req, nil
}

// ─────────────────────────────────────────────
// Response parsing
// ─────────────────────────────────────────────

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text,omitempty"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Input any    `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (r *anthropicResponse) toProviderResponse(latency time.Duration) *provider.CompletionResponse {
	resp := &provider.CompletionResponse{
		ID:      r.ID,
		Model:   r.Model,
		Latency: latency,
		Usage: provider.TokenUsage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			CacheReadTokens:  r.Usage.CacheReadInputTokens,
			CacheWriteTokens: r.Usage.CacheCreationInputTokens,
			TotalTokens:      r.Usage.InputTokens + r.Usage.OutputTokens,
		},
	}
	switch r.StopReason {
	case "tool_use":
		resp.FinishReason = provider.FinishReasonToolUse
	case "max_tokens":
		resp.FinishReason = provider.FinishReasonLength
	default:
		resp.FinishReason = provider.FinishReasonStop
	}

	for _, c := range r.Content {
		switch c.Type {
		case "text":
			resp.Content = append(resp.Content, provider.ContentPart{
				Type: provider.ContentTypeText,
				Text: c.Text,
			})
		case "tool_use":
			resp.Content = append(resp.Content, provider.ContentPart{
				Type:      provider.ContentTypeToolUse,
				ToolUseID: c.ID,
				ToolName:  c.Name,
				ToolInput: c.Input,
			})
		}
	}
	return resp
}

// ─────────────────────────────────────────────
// SSE streaming
// ─────────────────────────────────────────────

func (p *Provider) readSSE(ctx context.Context, resp *http.Response, ch chan<- provider.StreamEvent) {
	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- provider.StreamEvent{Type: provider.StreamEventError, Err: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "content_block_delta":
			var delta struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err == nil {
				if delta.Delta.Type == "text_delta" && delta.Delta.Text != "" {
					ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: delta.Delta.Text}
				}
			}
		case "message_delta":
			var delta struct {
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err == nil {
				ch <- provider.StreamEvent{
					Type:         provider.StreamEventDone,
					FinishReason: provider.FinishReason(delta.Delta.StopReason),
				}
			}
		case "message_stop":
			return
		case "error":
			ch <- provider.StreamEvent{
				Type: provider.StreamEventError,
				Err:  fmt.Errorf("anthropic stream error: %s", data),
			}
			return
		}
	}
}

// ─────────────────────────────────────────────
// Error handling
// ─────────────────────────────────────────────

func (p *Provider) parseError(resp *http.Response) error {
	var errBody struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	msg := errBody.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return &provider.ProviderError{
		Provider:   providerName,
		StatusCode: resp.StatusCode,
		Message:    msg,
		Retryable:  resp.StatusCode == 529 || resp.StatusCode == 429 || resp.StatusCode >= 500,
	}
}

// ─────────────────────────────────────────────
// Model catalog
// ─────────────────────────────────────────────

func anthropicModelCatalog(provName string) []provider.ModelInfo {
	visionTools := []provider.Capability{
		provider.CapabilityVision, provider.CapabilityTools,
		provider.CapabilityStreaming, provider.CapabilityJSON,
	}
	return []provider.ModelInfo{
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 8192, InputCostPerM: 3, OutputCostPerM: 15},
		{ID: "claude-opus-4-5", Name: "Claude Opus 4.5", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 32000, InputCostPerM: 15, OutputCostPerM: 75},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 16000, InputCostPerM: 3, OutputCostPerM: 15},
		{ID: "claude-haiku-3-5", Name: "Claude Haiku 3.5", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 8192, InputCostPerM: 0.25, OutputCostPerM: 1.25},
		{ID: "claude-opus-4-0", Name: "Claude Opus 4.0", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 32000, InputCostPerM: 15, OutputCostPerM: 75},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 8192, InputCostPerM: 3, OutputCostPerM: 15},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Provider: provName, Capabilities: visionTools, ContextWindow: 200000, MaxOutputTokens: 8192, InputCostPerM: 0.8, OutputCostPerM: 4},
	}
}
