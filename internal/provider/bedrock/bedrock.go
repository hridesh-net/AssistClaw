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
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
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
	// AccessKeyID optionally sets the AWS Access Key ID, bypassing ~/.aws/credentials.
	AccessKeyID string `yaml:"access_key_id"`
	// SecretAccessKey optionally sets the AWS Secret Access Key, bypassing ~/.aws/credentials.
	SecretAccessKey string `yaml:"secret_access_key"`
	// APIKey optionally sets the AWS Bedrock Bearer Token (API Key), bypassing SigV4.
	APIKey string `yaml:"api_key"`
	// DefaultModel is the default model ID (e.g. "anthropic.claude-3-5-sonnet-20241022-v2:0")
	DefaultModel string `yaml:"default_model"`
}

type apiKeyTransport struct {
	token string
	base  http.RoundTripper
}

func (t *apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := t.token
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
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
		// Inject the API key header via a custom transport
		transport := http.DefaultTransport
		opts = append(opts, awsconfig.WithHTTPClient(&http.Client{
			Transport: &apiKeyTransport{
				token: cfg.APIKey,
				base:  transport,
			},
		}))
	} else if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		// Use direct IAM access keys
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	} else if cfg.Profile != "" {
		// Use specific AWS profile
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		if cfg.APIKey != "" {
			awsCfg = aws.Config{
				Region:      cfg.Region,
				Credentials: aws.AnonymousCredentials{},
				HTTPClient: &http.Client{
					Transport: &apiKeyTransport{
						token: cfg.APIKey,
						base:  http.DefaultTransport,
					},
				},
			}
		} else if cfg.AccessKeyID != "" {
			awsCfg = aws.Config{
				Region: cfg.Region,
				Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
					return aws.Credentials{
						AccessKeyID:     cfg.AccessKeyID,
						SecretAccessKey: cfg.SecretAccessKey,
					}, nil
				}),
			}
		} else {
			return nil, fmt.Errorf("bedrock: load aws config: %w", err)
		}
	}

	return &Provider{
		cfg:    cfg,
		client: bedrockruntime.NewFromConfig(awsCfg),
	}, nil
}

func (p *Provider) Name() string { return providerName }

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

// ListModels returns the static Bedrock model catalog.
func (p *Provider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return bedrockModelCatalog(p.Name()), nil
}

// Complete performs a blocking Bedrock inference call using the unified Converse API.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}
	if model == "" {
		model = "anthropic.claude-3-5-haiku-20241022-v1:0"
	}

	messages, system, err := buildConverseMessages(req)
	if err != nil {
		return nil, fmt.Errorf("bedrock: build messages: %w", err)
	}

	toolConfig, err := buildConverseTools(req.Tools)
	if err != nil {
		return nil, fmt.Errorf("bedrock: build tools: %w", err)
	}

	input := &bedrockruntime.ConverseInput{
		ModelId:    aws.String(model),
		Messages:   messages,
		System:     system,
		ToolConfig: toolConfig,
	}
	if req.MaxTokens > 0 {
		input.InferenceConfig = &types.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(req.MaxTokens)),
		}
	}

	output, err := p.client.Converse(ctx, input)
	if err != nil {
		msg := "converse"
		if strings.Contains(err.Error(), "AccessDeniedException") || strings.Contains(err.Error(), "403") {
			msg = "access denied: ensure you have 'requested access' to this model in the AWS Bedrock Console for the configured region (" + p.cfg.Region + ")"
		}
		return nil, &provider.ProviderError{Provider: providerName, Message: msg, Err: err, Retryable: true}
	}

	resp := &provider.CompletionResponse{
		Model:   model,
		Latency: time.Since(start),
	}

	if content, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
		for _, b := range content.Value.Content {
			if t, ok := b.(*types.ContentBlockMemberText); ok {
				resp.Content = append(resp.Content, provider.ContentPart{Type: provider.ContentTypeText, Text: t.Value})
			} else if tu, ok := b.(*types.ContentBlockMemberToolUse); ok {
				resp.Content = append(resp.Content, provider.ContentPart{
					Type:      provider.ContentTypeToolUse,
					ToolUseID: *tu.Value.ToolUseId,
					ToolName:  *tu.Value.Name,
					ToolInput: tu.Value.Input,
				})
				resp.FinishReason = provider.FinishReasonToolUse
			}
		}
		if len(resp.Content) > 0 && resp.FinishReason == "" {
			resp.FinishReason = provider.FinishReasonStop
		}
	}

	if output.Usage != nil {
		resp.Usage = provider.TokenUsage{
			PromptTokens:     int(*output.Usage.InputTokens),
			CompletionTokens: int(*output.Usage.OutputTokens),
			TotalTokens:      int(*output.Usage.TotalTokens),
		}
	}

	return resp, nil
}

// Stream performs a streaming Bedrock inference using ConverseStream.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	messages, system, err := buildConverseMessages(req)
	if err != nil {
		return nil, err
	}

	toolConfig, err := buildConverseTools(req.Tools)
	if err != nil {
		return nil, err
	}

	input := &bedrockruntime.ConverseStreamInput{
		ModelId:    aws.String(model),
		Messages:   messages,
		System:     system,
		ToolConfig: toolConfig,
	}
	if req.MaxTokens > 0 {
		input.InferenceConfig = &types.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(req.MaxTokens)),
		}
	}

	output, err := p.client.ConverseStream(ctx, input)
	if err != nil {
		msg := "converse stream"
		if strings.Contains(err.Error(), "AccessDeniedException") || strings.Contains(err.Error(), "403") {
			msg = "access denied: ensure you have 'requested access' to this model in the AWS Bedrock Console for the configured region (" + p.cfg.Region + ")"
		}
		return nil, &provider.ProviderError{Provider: providerName, Message: msg, Err: err, Retryable: true}
	}

	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		stream := output.GetStream()
		defer stream.Close()

		// State for accumulating fragmented JSON tool calls during the stream
		type activeToolCall struct {
			ID        string
			Name      string
			Arguments string
		}
		activeToolCalls := make(map[string]*activeToolCall)

		for event := range stream.Events() {
			switch v := event.(type) {
			case *types.ConverseStreamOutputMemberContentBlockStart:
				if tu, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
					activeToolCalls[*tu.Value.ToolUseId] = &activeToolCall{
						ID:   *tu.Value.ToolUseId,
						Name: *tu.Value.Name,
					}
				}
			case *types.ConverseStreamOutputMemberContentBlockDelta:
				if d, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
					ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: d.Value}
				} else if tu, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
					for _, activeCall := range activeToolCalls { // Bedrock streams 1 tool delta block at a time
						activeCall.Arguments += *tu.Value.Input
					}
				}
			case *types.ConverseStreamOutputMemberMessageStop:
				// Emit tool calls before finishing stream
				for _, call := range activeToolCalls {
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
				ch <- provider.StreamEvent{Type: provider.StreamEventDone}
			case *types.ConverseStreamOutputMemberMetadata:
				if v.Value.Usage != nil {
					usage := &provider.TokenUsage{
						PromptTokens:     int(aws.ToInt32(v.Value.Usage.InputTokens)),
						CompletionTokens: int(aws.ToInt32(v.Value.Usage.OutputTokens)),
						TotalTokens:      int(aws.ToInt32(v.Value.Usage.TotalTokens)),
					}
					ch <- provider.StreamEvent{Type: provider.StreamEventDone, Usage: usage}
				}
			}
		}
	}()
	return ch, nil
}

func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return nil, fmt.Errorf("bedrock: embeddings not supported through this provider yet")
}

func (p *Provider) SupportsNativeStreaming() bool { return true }

// ─────────────────────────────────────────────
// Unified Converse Message Builders
// ─────────────────────────────────────────────

func buildConverseTools(reqTools []provider.ToolDef) (*types.ToolConfiguration, error) {
	if len(reqTools) == 0 {
		return nil, nil
	}
	var bTools []types.Tool
	for _, t := range reqTools {
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, err
		}

		var schemaMap map[string]any
		if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
			return nil, err
		}

		// Bedrock strictly requires "properties" to be present in the schema object,
		// even if the tool takes no arguments. Since our struct uses omitempty, we enforce it here.
		if schemaMap["properties"] == nil {
			schemaMap["properties"] = map[string]any{}
		}

		bTools = append(bTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(t.Name),
				Description: aws.String(t.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(schemaMap),
				},
			},
		})
	}
	return &types.ToolConfiguration{Tools: bTools}, nil
}

func buildConverseMessages(req *provider.CompletionRequest) ([]types.Message, []types.SystemContentBlock, error) {
	var messages []types.Message
	for _, m := range req.Messages {
		role := types.ConversationRoleUser
		if m.Role == provider.RoleAssistant {
			role = types.ConversationRoleAssistant
		}
		var content []types.ContentBlock
		for _, cp := range m.Content {
			if cp.Type == provider.ContentTypeText {
				text := cp.Text
				if strings.TrimSpace(text) == "" {
					text = "[No text content]"
				}
				content = append(content, &types.ContentBlockMemberText{Value: text})
			} else if cp.Type == provider.ContentTypeToolUse {
				content = append(content, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						Input:     document.NewLazyDocument(cp.ToolInput),
						Name:      aws.String(cp.ToolName),
						ToolUseId: aws.String(cp.ToolUseID),
					},
				})
			} else if cp.Type == provider.ContentTypeToolResult {
				resContent := cp.ToolResultContent
				if strings.TrimSpace(resContent) == "" {
					resContent = "[No content]"
				}
				status := types.ToolResultStatusSuccess
				if cp.ToolResultError {
					status = types.ToolResultStatusError
				}
				content = append(content, &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(cp.ToolResultID),
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{Value: resContent},
						},
						Status: status,
					},
				})
			}
		}
		if len(content) == 0 {
			content = append(content, &types.ContentBlockMemberText{Value: "[No text content]"})
		}
		messages = append(messages, types.Message{
			Role:    role,
			Content: content,
		})
	}

	var system []types.SystemContentBlock
	if req.SystemPrompt != "" {
		system = append(system, &types.SystemContentBlockMemberText{Value: req.SystemPrompt})
	}

	return messages, system, nil
}

// ─────────────────────────────────────────────
// Legacy Request builders (falling back if needed)
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
		{ID: "anthropic.claude-3-7-sonnet-20250219-v1:0", Name: "Claude 3.7 Sonnet (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		{ID: "anthropic.claude-opus-4-5-20251101-v1:0", Name: "Claude Opus 4.5 (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		{ID: "anthropic.claude-3-5-sonnet-20241022-v2:0", Name: "Claude 3.5 Sonnet (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},
		{ID: "anthropic.claude-3-5-haiku-20241022-v1:0", Name: "Claude 3.5 Haiku (Bedrock)", Provider: provName, Capabilities: vt, ContextWindow: 200000},

		// Amazon Nova on Bedrock
		{ID: "amazon.nova-pro-v1:0", Name: "Amazon Nova Pro", Provider: provName, Capabilities: vt, ContextWindow: 300000},
		{ID: "amazon.nova-lite-v1:0", Name: "Amazon Nova Lite", Provider: provName, Capabilities: vt, ContextWindow: 300000},
		{ID: "amazon.nova-micro-v1:0", Name: "Amazon Nova Micro", Provider: provName, Capabilities: t, ContextWindow: 128000},

		// Meta Llama on Bedrock
		{ID: "meta.llama-3-3-70b-instruct-v1:0", Name: "Llama 3.3 70B (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 128000},
		{ID: "meta.llama3-1-405b-instruct-v1:0", Name: "Llama 3.1 405B (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 128000},

		// Mistral on Bedrock
		{ID: "mistral.mistral-large-2402-v1:0", Name: "Mistral Large (Bedrock)", Provider: provName, Capabilities: t, ContextWindow: 32768},

		// Amazon Titan
		{ID: "amazon.titan-text-premier-v1:0", Name: "Titan Text Premier", Provider: provName, Capabilities: []provider.Capability{provider.CapabilityStreaming}, ContextWindow: 32000},
	}
}
