// Package openai implements the OpenAI and Azure OpenAI provider for AssistClaw.
// Supports both chat completions and streaming, vision, function/tool calling.
package openai

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
	defaultBaseURL    = "https://api.openai.com/v1"
	completionsPath   = "/chat/completions"
	embeddingsPath    = "/embeddings"
	modelsPath        = "/models"
	defaultTimeout    = 120 * time.Second
	providerName      = "openai"
	azureProviderName = "azure-openai"
)

// Config holds provider-level settings.
type Config struct {
	// APIKey is the OpenAI API key (or Azure API key).
	APIKey string `yaml:"api_key" json:"api_key"`
	// BaseURL overrides the OpenAI API base. For Azure: https://<resource>.openai.azure.com/openai
	BaseURL string `yaml:"base_url" json:"base_url"`
	// OrgID is optional for OpenAI organization-scoped requests.
	OrgID string `yaml:"org_id" json:"org_id"`
	// IsAzure enables Azure OpenAI path and header conventions.
	IsAzure bool `yaml:"is_azure" json:"is_azure"`
	// APIVersion is required for Azure (e.g. "2024-10-21").
	APIVersion string `yaml:"api_version" json:"api_version"`
	// DefaultModel used when not specified per-request.
	DefaultModel string `yaml:"default_model" json:"default_model"`
}

// Provider implements provider.Provider for OpenAI and Azure OpenAI.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new OpenAI provider from config.
func New(cfg Config) *Provider {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(base, "/")
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	if p.cfg.IsAzure {
		return azureProviderName
	}
	return providerName
}

func (p *Provider) Caps() provider.ProviderCaps {
	if p.cfg.IsAzure {
		return provider.CapsOpenAICompatDefault()
	}
	return provider.CapsOpenAI()
}

// HealthCheck verifies the API key and endpoint are reachable.
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

	// For OpenAI, we can check against ListModels, but since they have many models
	// and often add new ones, we'll allow it if it matches a known prefix or
	// we just let the request fail later for unknown ones but at least we tried.
	// Actually, strict validation is better for cloud providers.
	models, err := p.ListModels(ctx)
	if err != nil {
		return err // If we can't list, we can't validate (maybe API key issue)
	}
	for _, m := range models {
		if m.ID == modelID {
			return nil
		}
	}
	return &provider.ProviderError{
		Provider:   p.Name(),
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("model %q not found", modelID),
	}
}

// ListModels returns all available models from OpenAI.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := p.newRequest(ctx, http.MethodGet, modelsPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "list models", Err: err, Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &provider.ProviderError{Provider: p.Name(), StatusCode: resp.StatusCode, Message: "list models failed"}
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	var models []provider.ModelInfo
	for _, m := range response.Data {
		info := openAIModelToInfo(m.ID, p.Name())
		models = append(models, info)
	}
	return models, nil
}

// Complete performs a blocking chat completion.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, http.MethodPost, completionsPath, body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "complete", Err: err, Retryable: true}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(p.Name(), httpResp)
	}

	var response openAIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.toProviderResponse(p.Name(), time.Since(start)), nil
}

// Stream initiates a streaming chat completion.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, http.MethodPost, completionsPath, body)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Message: "stream", Err: err, Retryable: true}
	}
	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, parseErrorResponse(p.Name(), httpResp)
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		p.readSSE(ctx, httpResp, ch)
	}()
	return ch, nil
}

// Embed generates a vector representation using OpenAI's embedding API.
func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	if model == "" {
		model = "text-embedding-3-small"
	}

	body := struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{
		Model: model,
		Input: text,
	}

	rawBody, _ := json.Marshal(body)
	req, err := p.newRequest(ctx, http.MethodPost, embeddingsPath, rawBody)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(p.Name(), resp)
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
		return nil, fmt.Errorf("openai: no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

// SupportsNativeStreaming always returns true for OpenAI.
func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────

func (p *Provider) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := p.cfg.BaseURL + path
	if p.cfg.IsAzure {
		url += "?api-version=" + p.cfg.APIVersion
	}
	req, err := http.NewRequestWithContext(ctx, method, url,
		func() *bytes.Reader {
			if body == nil {
				return bytes.NewReader(nil)
			}
			return bytes.NewReader(body)
		}())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.IsAzure {
		req.Header.Set("api-key", p.cfg.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		if p.cfg.OrgID != "" {
			req.Header.Set("OpenAI-Organization", p.cfg.OrgID)
		}
	}
	return req, nil
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"` // string or []contentPart
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type contentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	StreamOpts  *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

func (p *Provider) buildRequestBody(req *provider.CompletionRequest, stream bool) ([]byte, error) {
	msgs := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, openAIMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	for _, m := range req.Messages {
		oaiMsg, err := convertMessage(m)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, oaiMsg)
	}

	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	body := openAIRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      stream,
	}

	if stream {
		body.StreamOpts = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, openAITool{
			Type: "function",
			Function: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  any    `json:"parameters"`
			}{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return json.Marshal(body)
}

func convertMessage(m provider.Message) (openAIMessage, error) {
	oai := openAIMessage{Role: string(m.Role)}

	// Simple text-only message → use string content for max compat.
	if len(m.Content) == 1 && m.Content[0].Type == provider.ContentTypeText {
		oai.Content = m.Content[0].Text
		return oai, nil
	}

	var parts []contentPart
	var toolCalls []openAIToolCall
	for _, cp := range m.Content {
		switch cp.Type {
		case provider.ContentTypeText:
			parts = append(parts, contentPart{Type: "text", Text: cp.Text})
		case provider.ContentTypeImage:
			if cp.ImageURL != "" {
				parts = append(parts, contentPart{
					Type: "image_url",
					ImageURL: &struct {
						URL    string `json:"url"`
						Detail string `json:"detail,omitempty"`
					}{URL: cp.ImageURL},
				})
			}
		case provider.ContentTypeToolUse:
			argsJSON, _ := json.Marshal(cp.ToolInput)
			toolCalls = append(toolCalls, openAIToolCall{
				ID:   cp.ToolUseID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: cp.ToolName, Arguments: string(argsJSON)},
			})
		case provider.ContentTypeAudio:
			// Fallback: If transcription failed, at least indicate there is audio
			parts = append(parts, contentPart{Type: "text", Text: "[Audio Received]"})
		case provider.ContentTypeToolResult:
			oai.Role = "tool"
			oai.Content = cp.ToolResultContent
			oai.ToolCallID = cp.ToolResultID
			return oai, nil
		}
	}

	if len(toolCalls) > 0 {
		oai.ToolCalls = toolCalls
		oai.Content = nil
	} else {
		oai.Content = parts
	}
	return oai, nil
}

// ─────────────────────────────────────────────
// Response parsing
// ─────────────────────────────────────────────

type openAIResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (r *openAIResponse) toProviderResponse(provName string, latency time.Duration) *provider.CompletionResponse {
	resp := &provider.CompletionResponse{
		ID:      r.ID,
		Model:   r.Model,
		Latency: latency,
		Usage: provider.TokenUsage{
			PromptTokens:     r.Usage.PromptTokens,
			CompletionTokens: r.Usage.CompletionTokens,
			TotalTokens:      r.Usage.TotalTokens,
		},
	}
	if len(r.Choices) > 0 {
		choice := r.Choices[0]
		resp.FinishReason = provider.FinishReason(choice.FinishReason)
		// Parse content from OpenAI message back to provider format
		switch v := choice.Message.Content.(type) {
		case string:
			resp.Content = append(resp.Content, provider.ContentPart{Type: provider.ContentTypeText, Text: v})
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
	return resp
}

// ─────────────────────────────────────────────
// SSE streaming
// ─────────────────────────────────────────────

func (p *Provider) readSSE(ctx context.Context, resp *http.Response, ch chan<- provider.StreamEvent) {
	scanner := bufio.NewScanner(resp.Body)
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
					Content   string           `json:"content"`
					ToolCalls []openAIToolCall `json:"tool_calls"`
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

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: choice.Delta.Content}
			}
			for _, tc := range choice.Delta.ToolCalls {
				var args any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				ch <- provider.StreamEvent{
					Type: provider.StreamEventToolUse,
					ToolUse: &provider.ContentPart{
						Type:      provider.ContentTypeToolUse,
						ToolUseID: tc.ID,
						ToolName:  tc.Function.Name,
						ToolInput: args,
					},
				}
			}
			if choice.FinishReason != "" {
				ch <- provider.StreamEvent{
					Type:         provider.StreamEventDone,
					FinishReason: provider.FinishReason(choice.FinishReason),
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- provider.StreamEvent{Type: provider.StreamEventError, Err: err}
	}
}

// ─────────────────────────────────────────────
// Error parsing
// ─────────────────────────────────────────────

func parseErrorResponse(provName string, resp *http.Response) error {
	var errBody struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
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

// ─────────────────────────────────────────────
// Model catalog (well-known models with caps)
// ─────────────────────────────────────────────

func openAIModelToInfo(id, provName string) provider.ModelInfo {
	info := provider.ModelInfo{
		ID:       id,
		Name:     id,
		Provider: provName,
	}

	// Exact name mapping for common/latest models
	exactNames := map[string]string{
		"gpt-4o":                     "GPT-4o",
		"chatgpt-4o-latest":          "ChatGPT 4o Latest",
		"gpt-4o-mini":                "GPT-4o Mini",
		"gpt-4o-2024-11-20":          "GPT-4o (Nov 2024)",
		"gpt-4o-2024-08-06":          "GPT-4o (Aug 2024)",
		"gpt-4o-2024-05-13":          "GPT-4o (May 2024)",
		"gpt-4o-audio-preview":       "GPT-4o Audio Preview",
		"gpt-4o-mini-2024-07-18":     "GPT-4o Mini (Jul 2024)",
		"o1":                         "o1",
		"o1-2024-12-17":              "o1 (Dec 2024)",
		"o1-preview":                 "o1 Preview",
		"o1-preview-2024-09-12":      "o1 Preview (Sep 2024)",
		"o1-mini":                    "o1 Mini",
		"o1-mini-2024-09-12":         "o1 Mini (Sep 2024)",
		"o3-mini":                    "o3 Mini",
		"o3-mini-2025-01-31":         "o3 Mini (Jan 2025)",
		"gpt-4.5-preview":            "GPT-4.5 Preview",
		"gpt-4.5-preview-2025-02-27": "GPT-4.5 Preview (Feb 2025)",
		"gpt-4-turbo":                "GPT-4 Turbo",
		"gpt-4-turbo-2024-04-09":     "GPT-4 Turbo (Apr 2024)",
		"gpt-4-0125-preview":         "GPT-4 (Jan 2024)",
		"gpt-4-1106-preview":         "GPT-4 (Nov 2023)",
		"gpt-4-0613":                 "GPT-4 (Jun 2023)",
		"gpt-4":                      "GPT-4",
		"gpt-3.5-turbo-0125":         "GPT-3.5 Turbo (Jan 2024)",
		"gpt-3.5-turbo":              "GPT-3.5 Turbo",
	}

	if name, ok := exactNames[id]; ok {
		info.Name = name
	} else if strings.HasPrefix(strings.ToLower(id), "ft:") {
		info.Name = "Fine-Tuned: " + id[3:]
	}

	// Apply known caps for well-known model families
	lower := strings.ToLower(id)
	if strings.Contains(lower, "gpt-4.5") {
		info.Capabilities = []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming, provider.CapabilityJSON}
		info.ContextWindow = 128000
		info.MaxOutputTokens = 16384
	} else if strings.Contains(lower, "gpt-4o") || strings.Contains(lower, "gpt-4-vision") {
		info.Capabilities = []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming, provider.CapabilityJSON}
		info.ContextWindow = 128000
		info.MaxOutputTokens = 16384
	} else if strings.Contains(lower, "gpt-4") {
		info.Capabilities = []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming, provider.CapabilityJSON}
		info.ContextWindow = 128000
		info.MaxOutputTokens = 4096
	} else if strings.Contains(lower, "gpt-3.5") {
		info.Capabilities = []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
		info.ContextWindow = 16385
		info.MaxOutputTokens = 4096
	} else if strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4") {
		info.Capabilities = []provider.Capability{provider.CapabilityReasoning, provider.CapabilityTools, provider.CapabilityStreaming}
		info.ContextWindow = 200000
		info.MaxOutputTokens = 100000
	}
	return info
}
