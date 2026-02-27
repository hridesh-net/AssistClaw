// Package openaicompat provides a shared base for OpenAI-compatible APIs.
// Used by: vLLM, LM Studio, Groq, Mistral, Together, NVIDIA, OpenRouter,
// Cohere (v2), HuggingFace TGI, and Cloudflare AI Gateway.
package openaicompat

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

const defaultTimeout = 120 * time.Second

// Config holds settings for an OpenAI-compatible provider.
type Config struct {
	Name         string `yaml:"name" json:"name"`
	BaseURL      string `yaml:"base_url" json:"base_url"`
	APIKey       string `yaml:"api_key" json:"api_key"`
	DefaultModel string `yaml:"default_model" json:"default_model"`
	// ExtraHeaders are additional HTTP headers sent with every request.
	ExtraHeaders map[string]string `yaml:"extra_headers" json:"extra_headers"`
	// StaticModels is used for providers where /v1/models is unreliable or
	// returns different data than what the provider advertises.
	StaticModels []provider.ModelInfo `yaml:"-" json:"-"`
	// DiscoverModels controls whether to call /v1/models on startup.
	DiscoverModels bool `yaml:"discover_models" json:"discover_models"`
}

// Provider implements provider.Provider using the OpenAI chat completions API.
// This is the base for all OpenAI-compatible backends.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new OpenAI-compatible provider.
func New(cfg Config) *Provider {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Provider{cfg: cfg, client: &http.Client{Timeout: defaultTimeout}}
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) HealthCheck(ctx context.Context) error {
	req, err := p.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return &provider.ProviderError{Provider: p.Name(), Message: "health check", Err: err, Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &provider.ProviderError{Provider: p.Name(), StatusCode: resp.StatusCode, Message: "health check failed"}
	}
	return nil
}

// ListModels returns available models. Uses static catalog if configured,
// otherwise queries /v1/models.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if len(p.cfg.StaticModels) > 0 && !p.cfg.DiscoverModels {
		return p.cfg.StaticModels, nil
	}

	req, err := p.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// Fall back to static catalog if dynamic discovery fails.
		if len(p.cfg.StaticModels) > 0 {
			return p.cfg.StaticModels, nil
		}
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "list models", Err: err, Retryable: true}
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Data) == 0 {
		return p.cfg.StaticModels, nil
	}

	models := make([]provider.ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, provider.ModelInfo{
			ID:              m.ID,
			Name:            m.ID,
			Provider:        p.Name(),
			Capabilities:    []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools},
			ContextWindow:   128000,
			MaxOutputTokens: 8192,
			Local:           isLocalURL(p.cfg.BaseURL),
		})
	}
	return models, nil
}

// Complete performs a blocking chat completion.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	body, err := p.buildBody(req, false)
	if err != nil {
		return nil, err
	}
	httpReq, err := p.newRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "complete", Err: err, Retryable: true}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, parseError(p.Name(), httpResp)
	}

	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return nil, err
	}

	resp := &provider.CompletionResponse{
		ID:      response.ID,
		Model:   response.Model,
		Latency: time.Since(start),
		Usage: provider.TokenUsage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}
	if len(response.Choices) > 0 {
		choice := response.Choices[0]
		resp.FinishReason = provider.FinishReason(choice.FinishReason)
		if choice.Message.Content != "" {
			resp.Content = append(resp.Content, provider.ContentPart{
				Type: provider.ContentTypeText,
				Text: choice.Message.Content,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			var args any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			resp.Content = append(resp.Content, provider.ContentPart{
				Type:      provider.ContentTypeToolUse,
				ToolUseID: tc.ID,
				ToolName:  tc.Function.Name,
				ToolInput: args,
			})
		}
	}
	return resp, nil
}

// Stream initiates a streaming chat completion.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	body, err := p.buildBody(req, true)
	if err != nil {
		return nil, err
	}
	httpReq, err := p.newRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "stream", Err: err, Retryable: true}
	}
	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, parseError(p.Name(), httpResp)
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		readSSE(ctx, httpResp, ch)
	}()
	return ch, nil
}

func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	if model == "" {
		model = p.cfg.DefaultModel
	}
	body := struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{
		Model: model,
		Input: text,
	}
	rawBody, _ := json.Marshal(body)
	req, err := p.newRequest(ctx, http.MethodPost, "/embeddings", rawBody)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(p.Name(), resp)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openaicompat: no embedding returned")
	}
	return result.Data[0].Embedding, nil
}

func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────

func (p *Provider) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := p.cfg.BaseURL + "/v1" + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	for k, v := range p.cfg.ExtraHeaders {
		req.Header.Set(k, v)
	}
	return req, nil
}

type chatMessage struct {
	Role       string `json:"role"`
	Content    any    `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolDef     `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	StreamOpts  *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

func (p *Provider) buildBody(req *provider.CompletionRequest, stream bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	var messages []chatMessage
	if req.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		cm := chatMessage{Role: string(m.Role)}
		if len(m.Content) == 1 && m.Content[0].Type == provider.ContentTypeText {
			cm.Content = m.Content[0].Text
		} else {
			cm.Content = m.Content
		}
		if m.Content[0].Type == provider.ContentTypeToolResult {
			cm.ToolCallID = m.Content[0].ToolResultID
			cm.Content = m.Content[0].ToolResultContent
		}
		messages = append(messages, cm)
	}

	body := chatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      stream,
	}
	if stream {
		body.StreamOpts = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
	}
	for _, t := range req.Tools {
		var td toolDef
		td.Type = "function"
		td.Function.Name = t.Name
		td.Function.Description = t.Description
		td.Function.Parameters = t.InputSchema
		body.Tools = append(body.Tools, td)
	}
	return json.Marshal(body)
}

func readSSE(ctx context.Context, resp *http.Response, ch chan<- provider.StreamEvent) {
	scanner := bufio.NewScanner(resp.Body)

	// State for accumulating fragmented JSON tool calls during the stream
	type activeToolCall struct {
		ID        string
		Name      string
		Arguments string
	}
	var activeToolCalls []*activeToolCall

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- provider.StreamEvent{Type: provider.StreamEventError, Err: ctx.Err()}
			return
		default:
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- provider.StreamEvent{Type: provider.StreamEventDone}
			return
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			ch <- provider.StreamEvent{
				Type: provider.StreamEventDone,
				Usage: &provider.TokenUsage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				},
			}
			return
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: c.Delta.Content}
			}

			// Capture streamed tool call segments
			for _, tc := range c.Delta.ToolCalls {
				idx := tc.Index
				if len(activeToolCalls) <= idx {
					// Grow slice if needed (usually streams in order)
					newCalls := make([]*activeToolCall, idx+1)
					copy(newCalls, activeToolCalls)
					activeToolCalls = newCalls
				}
				if activeToolCalls[idx] == nil {
					activeToolCalls[idx] = &activeToolCall{}
				}

				call := activeToolCalls[idx]
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					call.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					call.Arguments += tc.Function.Arguments
				}
			}

			if c.FinishReason != "" {
				// We've finished, emit any accumulated tool calls first
				for _, call := range activeToolCalls {
					if call == nil || call.Name == "" {
						continue
					}
					var args any
					_ = json.Unmarshal([]byte(call.Arguments), &args)

					ch <- provider.StreamEvent{
						Type: provider.StreamEventToolUse,
						ToolUse: &provider.ContentPart{
							Type:      provider.ContentTypeToolUse,
							ToolUseID: call.ID,
							ToolName:  call.Name,
							ToolInput: args,
						},
					}
				}

				ch <- provider.StreamEvent{Type: provider.StreamEventDone, FinishReason: provider.FinishReason(c.FinishReason)}
			}
		}
	}
}

func parseError(provName string, resp *http.Response) error {
	var errBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	msg := errBody.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return &provider.ProviderError{
		Provider:   provName,
		StatusCode: resp.StatusCode,
		Message:    msg,
		Retryable:  resp.StatusCode == 429 || resp.StatusCode >= 500,
	}
}

func isLocalURL(u string) bool {
	return strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") || strings.Contains(u, "0.0.0.0")
}
