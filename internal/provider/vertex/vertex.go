// Package vertex implements the Google Vertex AI provider for AssistClaw.
package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"os"

	"github.com/assistclaw/assistclaw/internal/provider"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	providerName   = "vertex"
	defaultTimeout = 120 * time.Second
)

// Config holds settings for Vertex AI.
type Config struct {
	ProjectID    string `yaml:"project_id"`
	Location     string `yaml:"location"`
	Credentials  string `yaml:"credentials"` // path to service account JSON
	DefaultModel string `yaml:"default_model"`
}

// Provider implements provider.Provider for Vertex AI.
type Provider struct {
	cfg    Config
	client *http.Client
	ts     oauth2.TokenSource
}

// New creates a new Vertex AI provider.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Location == "" {
		cfg.Location = "us-central1"
	}

	var ts oauth2.TokenSource

	if cfg.Credentials != "" {
		data, err := os.ReadFile(cfg.Credentials)
		if err != nil {
			return nil, fmt.Errorf("vertex: read credentials: %w", err)
		}
		creds, err := google.CredentialsFromJSON(ctx, data, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, fmt.Errorf("vertex: parse credentials: %w", err)
		}
		ts = creds.TokenSource
	} else {
		// Try default credentials
		creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err == nil {
			ts = creds.TokenSource
		}
	}

	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: defaultTimeout},
		ts:     ts,
	}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Caps() provider.ProviderCaps { return provider.CapsGemini() }

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

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	// Google doesn't have a simple /models endpoint like OpenAI for Vertex AI easily.
	// We return a static catalog of common Gemini models.
	return []provider.ModelInfo{
		{
			ID: "gemini-2.0-flash-exp", Name: "Gemini 2.0 Flash Exp", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   1000000,
			MaxOutputTokens: 8192,
		},
		{
			ID: "gemini-2.0-flash-thinking-exp-01-21", Name: "Gemini 2.0 Flash Thinking", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityReasoning, provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   1000000,
			MaxOutputTokens: 8192,
		},
		{
			ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   1000000,
			MaxOutputTokens: 8192,
		},
		{
			ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   2000000,
			MaxOutputTokens: 8192,
		},
		{
			ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   2000000,
			MaxOutputTokens: 8192,
		},
		{
			ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", Provider: providerName,
			Capabilities:    []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming},
			ContextWindow:   1000000,
			MaxOutputTokens: 8192,
		},
	}, nil
}

func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}
	if model == "" {
		model = "gemini-1.5-flash"
	}

	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		p.cfg.Location, p.cfg.ProjectID, p.cfg.Location, model)

	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	token, err := p.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("vertex: get token: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		var errData any
		_ = json.NewDecoder(httpResp.Body).Decode(&errData)
		return nil, fmt.Errorf("vertex: http %d: %v", httpResp.StatusCode, errData)
	}

	var vertexResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&vertexResp); err != nil {
		return nil, err
	}

	resp := &provider.CompletionResponse{
		Model:   model,
		Latency: time.Since(start),
		Usage: provider.TokenUsage{
			PromptTokens:     vertexResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: vertexResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      vertexResp.UsageMetadata.TotalTokenCount,
		},
	}

	if len(vertexResp.Candidates) > 0 {
		c := vertexResp.Candidates[0]
		for _, part := range c.Content.Parts {
			if part.Text != "" {
				resp.Content = append(resp.Content, provider.ContentPart{Type: provider.ContentTypeText, Text: part.Text})
			}
		}
		resp.FinishReason = provider.FinishReason(strings.ToLower(c.FinishReason))
	}

	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	// TODO: Implement Vertex streaming (Server-Sent Events or chunked JSON)
	// For now, return error as not implemented or fallback to blocking.
	return nil, fmt.Errorf("vertex: streaming not yet implemented")
}

func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return nil, fmt.Errorf("vertex: embeddings not supported through this provider yet")
}

func (p *Provider) buildRequestBody(req *provider.CompletionRequest) ([]byte, error) {
	type part struct {
		Text string `json:"text,omitempty"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}
	type request struct {
		Contents []content `json:"contents"`
		System   *content  `json:"system_instruction,omitempty"`
	}

	var vertexReq request
	if req.SystemPrompt != "" {
		vertexReq.System = &content{
			Parts: []part{{Text: req.SystemPrompt}},
		}
	}

	for _, m := range req.Messages {
		role := string(m.Role)
		if role == "assistant" {
			role = "model"
		}
		var parts []part
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				parts = append(parts, part{Text: cp.Text})
			}
		}
		vertexReq.Contents = append(vertexReq.Contents, content{Role: role, Parts: parts})
	}

	return json.Marshal(vertexReq)
}
