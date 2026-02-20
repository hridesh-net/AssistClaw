// Package bedrock implements the AWS Bedrock provider for AssistClaw.
// Supports Claude (Anthropic), Titan, Llama, Mistral, and Cohere models
// through the Bedrock Runtime API using the AWS SDK v2.
package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/assistclaw/assistclaw/internal/provider"
)

const providerName = "bedrock"

// Config holds AWS Bedrock provider settings.
type Config struct {
	// Region is the AWS region (e.g. "us-east-1").
	Region string `yaml:"region"`
	// Profile is the named AWS profile (~/.aws/credentials). Uses default if empty.
	Profile string `yaml:"profile"`
	// APIKey optionally sets the AWS Bedrock Bearer Token (API Key), bypassing SigV4.
	APIKey string `yaml:"api_key"`
	// DefaultModel is the default model ID (e.g. "anthropic.claude-3-5-sonnet-20241022-v2:0")
	DefaultModel string `yaml:"default_model"`
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// Provider implements provider.Provider for AWS Bedrock.
type Provider struct {
	cfg    Config
	client *bedrockruntime.Client
}

// New creates a new Bedrock provider. Loads credentials from the standard
// AWS credential chain: env vars → ~/.aws/credentials → IAM role.
func New(cfg Config) (*Provider, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.APIKey != "" {
		// Disable AWS SigV4 by using anonymous credentials
		opts = append(opts, awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
		// Inject the Bearer token header via a custom transport
		transport := http.DefaultTransport
		opts = append(opts, awsconfig.WithHTTPClient(&http.Client{
			Transport: &bearerTransport{
				token: cfg.APIKey,
				base:  transport,
			},
		}))
	} else if cfg.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("bedrock: load aws config: %w", err)
	}

	return &Provider{
		cfg:    cfg,
		client: bedrockruntime.NewFromConfig(awsCfg),
	}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) HealthCheck(ctx context.Context) error {
	// Minimal call to verify credentials are valid.
	_, err := p.Complete(ctx, &provider.CompletionRequest{
		Model:     orDefault(p.cfg.DefaultModel, "anthropic.claude-3-haiku-20240307-v1:0"),
		Messages:  []provider.Message{provider.NewTextMessage(provider.RoleUser, "ping")},
		MaxTokens: 1,
	})
	return err
}

// ListModels returns the static Bedrock model catalog.
func (p *Provider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return bedrockModelCatalog(p.Name()), nil
}

// Complete performs a blocking Bedrock inference call. Automatically selects
// the correct request format based on the model family.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}
	if model == "" {
		model = "anthropic.claude-3-5-haiku-20241022-v1:0"
	}

	// Route to correct format based on model family prefix.
	var bodyBytes []byte
	var err error

	switch {
	case strings.HasPrefix(model, "anthropic."):
		bodyBytes, err = buildAnthropicBody(req)
	case strings.HasPrefix(model, "meta."):
		bodyBytes, err = buildMetaBody(req)
	case strings.HasPrefix(model, "mistral."):
		bodyBytes, err = buildMistralBody(req)
	case strings.HasPrefix(model, "amazon."):
		bodyBytes, err = buildTitanBody(req)
	default:
		bodyBytes, err = buildAnthropicBody(req) // safe default
	}
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}

	output, err := p.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "invoke model", Err: err, Retryable: true}
	}

	resp, err := parseBedrockResponse(model, output.Body)
	if err != nil {
		return nil, err
	}
	resp.Model = model
	resp.Latency = time.Since(start)
	return resp, nil
}

// Stream performs a streaming Bedrock inference using InvokeModelWithResponseStream.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	var bodyBytes []byte
	var err error
	switch {
	case strings.HasPrefix(model, "anthropic."):
		bodyBytes, err = buildAnthropicBody(req)
	default:
		bodyBytes, err = buildAnthropicBody(req)
	}
	if err != nil {
		return nil, err
	}

	output, err := p.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, &provider.ProviderError{Provider: providerName, Message: "stream invoke", Err: err, Retryable: true}
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		stream := output.GetStream()
		defer stream.Close()
		for event := range stream.Events() {
			if v, ok := event.(*types.ResponseStreamMemberChunk); ok {
				var chunk struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				if err := json.Unmarshal(v.Value.Bytes, &chunk); err == nil {
					if chunk.Type == "content_block_delta" && chunk.Delta.Text != "" {
						ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: chunk.Delta.Text}
					}
					if chunk.Type == "message_stop" {
						ch <- provider.StreamEvent{Type: provider.StreamEventDone}
					}
				}
			}
		}
	}()
	return ch, nil
}

func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Request builders per model family
// ─────────────────────────────────────────────

func buildAnthropicBody(req *provider.CompletionRequest) ([]byte, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []msg
	for _, m := range req.Messages {
		content := ""
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				content += cp.Text
			}
		}
		msgs = append(msgs, msg{Role: string(m.Role), Content: content})
	}
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 4096
	}
	body := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"messages":          msgs,
		"max_tokens":        maxTok,
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}
	return json.Marshal(body)
}

func buildMetaBody(req *provider.CompletionRequest) ([]byte, error) {
	var prompt strings.Builder
	if req.SystemPrompt != "" {
		prompt.WriteString("<|begin_of_text|><|start_header_id|>system<|end_header_id|>\n")
		prompt.WriteString(req.SystemPrompt)
		prompt.WriteString("<|eot_id|>")
	}
	for _, m := range req.Messages {
		prompt.WriteString(fmt.Sprintf("<|start_header_id|>%s<|end_header_id|>\n", m.Role))
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				prompt.WriteString(cp.Text)
			}
		}
		prompt.WriteString("<|eot_id|>")
	}
	prompt.WriteString("<|start_header_id|>assistant<|end_header_id|>")
	return json.Marshal(map[string]any{"prompt": prompt.String(), "max_gen_len": req.MaxTokens})
}

func buildMistralBody(req *provider.CompletionRequest) ([]byte, error) {
	var prompt strings.Builder
	for _, m := range req.Messages {
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				prompt.WriteString(cp.Text)
			}
		}
	}
	return json.Marshal(map[string]any{"prompt": prompt.String(), "max_tokens": req.MaxTokens})
}

func buildTitanBody(req *provider.CompletionRequest) ([]byte, error) {
	var prompt strings.Builder
	for _, m := range req.Messages {
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				prompt.WriteString(cp.Text)
			}
		}
	}
	return json.Marshal(map[string]any{
		"inputText":            prompt.String(),
		"textGenerationConfig": map[string]any{"maxTokenCount": req.MaxTokens},
	})
}

func parseBedrockResponse(modelID string, body []byte) (*provider.CompletionResponse, error) {
	switch {
	case strings.HasPrefix(modelID, "anthropic."):
		var r struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		text := ""
		for _, c := range r.Content {
			text += c.Text
		}
		return &provider.CompletionResponse{
			Content:      []provider.ContentPart{{Type: provider.ContentTypeText, Text: text}},
			FinishReason: provider.FinishReasonStop,
			Usage: provider.TokenUsage{
				PromptTokens: r.Usage.InputTokens, CompletionTokens: r.Usage.OutputTokens,
				TotalTokens: r.Usage.InputTokens + r.Usage.OutputTokens,
			},
		}, nil

	default:
		var r struct {
			Outputs []struct {
				Text string `json:"text"`
			} `json:"outputs"`
			Results []struct {
				OutputText string `json:"outputText"`
			} `json:"results"`
			Generation string `json:"generation"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		text := r.Generation
		for _, o := range r.Outputs {
			text += o.Text
		}
		for _, res := range r.Results {
			text += res.OutputText
		}
		return &provider.CompletionResponse{
			Content:      []provider.ContentPart{{Type: provider.ContentTypeText, Text: text}},
			FinishReason: provider.FinishReasonStop,
		}, nil
	}
}

func orDefault(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

func bedrockModelCatalog(provName string) []provider.ModelInfo {
	vt := []provider.Capability{provider.CapabilityVision, provider.CapabilityTools, provider.CapabilityStreaming}
	t := []provider.Capability{provider.CapabilityTools, provider.CapabilityStreaming}
	return []provider.ModelInfo{
		// Anthropic on Bedrock
		{ID: "anthropic.claude-opus-4-5-20251101-v1:0", Name: "Claude Opus 4.5 (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		{ID: "anthropic.claude-3-5-sonnet-20241022-v2:0", Name: "Claude 3.5 Sonnet (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		{ID: "anthropic.claude-3-5-haiku-20241022-v1:0", Name: "Claude 3.5 Haiku (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		// Meta Llama on Bedrock
		{ID: "meta.llama3-3-70b-instruct-v1:0", Name: "Llama 3.3 70B (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 128000},
		{ID: "meta.llama3-1-405b-instruct-v1:0", Name: "Llama 3.1 405B (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 128000},
		// Mistral on Bedrock
		{ID: "mistral.mistral-large-2402-v1:0", Name: "Mistral Large (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 32768},
		// Amazon Titan
		{ID: "amazon.titan-text-premier-v1:0", Name: "Titan Text Premier", Provider: provName, Capabilities: []provider.Capability{provider.CapabilityStreaming}, ContextWindow: 32000},
	}
}
