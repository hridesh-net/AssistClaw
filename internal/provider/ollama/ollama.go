// Package ollama implements the Ollama local LLM provider for AssistClaw.
// Supports model auto-discovery, streaming, and native Ollama JSON format.
package ollama

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
	defaultBaseURL = "http://localhost:11434"
	providerName   = "ollama"
	defaultTimeout = 300 * time.Second // local models can be slow to first token
)

// Config holds Ollama provider settings.
type Config struct {
	BaseURL      string `yaml:"base_url" json:"base_url"`
	DefaultModel string `yaml:"default_model" json:"default_model"`
}

// Provider implements provider.Provider for Ollama.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new Ollama provider.
func New(cfg Config) *Provider {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(base, "/")
	return &Provider{cfg: cfg, client: &http.Client{Timeout: defaultTimeout}}
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/api/version", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return &provider.ProviderError{Provider: providerName, Message: "ollama not reachable", Err: err, Retryable: true}
	}
	defer resp.Body.Close()
	return nil
}

// ListModels auto-discovers all pulled models from the local Ollama instance.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "list models", Err: err, Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: list models: http %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				ParameterSize string `json:"parameter_size"`
				Family        string `json:"family"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]provider.ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		lower := strings.ToLower(m.Name)
		caps := []provider.Capability{provider.CapabilityStreaming, provider.CapabilityTools}
		if strings.Contains(lower, "vision") || strings.Contains(lower, "llava") ||
			strings.Contains(lower, "bakllava") || strings.Contains(lower, "moondream") {
			caps = append(caps, provider.CapabilityVision)
		}
		if strings.Contains(lower, "r1") || strings.Contains(lower, "reasoning") || strings.Contains(lower, "qwq") {
			caps = append(caps, provider.CapabilityReasoning)
		}
		models = append(models, provider.ModelInfo{
			ID:              m.Name,
			Name:            m.Name,
			Provider:        providerName,
			Capabilities:    caps,
			Local:           true,
			ContextWindow:   128000,
			MaxOutputTokens: 8192,
		})
	}
	return models, nil
}

// Complete performs a blocking chat completion via Ollama's /api/chat endpoint.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "complete", Err: err, Retryable: true}
	}
	defer httpResp.Body.Close()

	var result ollamaResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.toProviderResponse(time.Since(start)), nil
}

// Stream initiates a streaming Ollama chat completion.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "stream", Err: err, Retryable: true}
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		p.readNDJSON(ctx, httpResp, ch)
	}()
	return ch, nil
}

func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	if model == "" {
		model = p.cfg.DefaultModel
	}
	body := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}{
		Model:  model,
		Prompt: text,
	}
	rawBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api/embeddings", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: embedding: http %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Request / Response types
// ─────────────────────────────────────────────

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // base64
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done            bool  `json:"done"`
	PromptEvalCount int   `json:"prompt_eval_count"`
	EvalCount       int   `json:"eval_count"`
	TotalDuration   int64 `json:"total_duration"`
}

func (r *ollamaResponse) toProviderResponse(latency time.Duration) *provider.CompletionResponse {
	return &provider.CompletionResponse{
		Model:        r.Model,
		FinishReason: provider.FinishReasonStop,
		Content: []provider.ContentPart{{
			Type: provider.ContentTypeText, Text: r.Message.Content,
		}},
		Usage: provider.TokenUsage{
			PromptTokens:     r.PromptEvalCount,
			CompletionTokens: r.EvalCount,
			TotalTokens:      r.PromptEvalCount + r.EvalCount,
		},
		Latency: latency,
	}
}

func (p *Provider) buildRequestBody(req *provider.CompletionRequest, stream bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	var messages []ollamaMessage
	if req.SystemPrompt != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msg := ollamaMessage{Role: string(m.Role)}
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				msg.Content += cp.Text
			}
		}
		messages = append(messages, msg)
	}

	opts := map[string]any{}
	if req.MaxTokens > 0 {
		opts["num_predict"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		opts["temperature"] = req.Temperature
	}

	return json.Marshal(ollamaRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
		Options:  opts,
	})
}

func (p *Provider) readNDJSON(ctx context.Context, resp *http.Response, ch chan<- provider.StreamEvent) {
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- provider.StreamEvent{Type: provider.StreamEventError, Err: ctx.Err()}
			return
		default:
		}

		var chunk ollamaResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Message.Content != "" {
			ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: chunk.Message.Content}
		}
		if chunk.Done {
			ch <- provider.StreamEvent{
				Type:         provider.StreamEventDone,
				FinishReason: provider.FinishReasonStop,
				Usage: &provider.TokenUsage{
					PromptTokens:     chunk.PromptEvalCount,
					CompletionTokens: chunk.EvalCount,
					TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
				},
			}
			return
		}
	}
}
