// Package embedproviders wires all embedding provider implementations.
package embedproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/embeddings"
)

// ─────────────────────────────────────────────
// OpenAI Embeddings
// ─────────────────────────────────────────────

type openAIEmbedder struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAI creates an OpenAI embedding provider.
func NewOpenAI(apiKey, baseURL string) embeddings.Embedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &openAIEmbedder{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 30 * time.Second}}
}

func (e *openAIEmbedder) Name() string         { return "openai" }
func (e *openAIEmbedder) DefaultModel() string { return "text-embedding-3-small" }

func (e *openAIEmbedder) HealthCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (e *openAIEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Provider: "openai", Dimensions: 1536, MaxInputTokens: 8191, CostPerMTokens: 0.02},
		{ID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Provider: "openai", Dimensions: 3072, MaxInputTokens: 8191, CostPerMTokens: 0.13},
		{ID: "text-embedding-ada-002", Name: "Text Embedding Ada 002", Provider: "openai", Dimensions: 1536, MaxInputTokens: 8191, CostPerMTokens: 0.1},
	}, nil
}

func (e *openAIEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"input": req.Texts,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embeddings: http %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		vecs[d.Index] = d.Embedding
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	return &embeddings.EmbedResponse{
		Embeddings: vecs,
		Dimensions: dim,
		Model:      model,
		TokensUsed: result.Usage.TotalTokens,
	}, nil
}

// ─────────────────────────────────────────────
// Ollama Embeddings (local)
// ─────────────────────────────────────────────

type ollamaEmbedder struct {
	baseURL string
	client  *http.Client
}

// NewOllama creates an Ollama embedding provider for local inference.
func NewOllama(baseURL string) embeddings.Embedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &ollamaEmbedder{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 120 * time.Second}}
}

func (e *ollamaEmbedder) Name() string         { return "ollama-embed" }
func (e *ollamaEmbedder) DefaultModel() string { return "nomic-embed-text" }

func (e *ollamaEmbedder) HealthCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/api/version", nil)
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (e *ollamaEmbedder) ListModels(ctx context.Context) ([]embeddings.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return staticOllamaEmbedModels(), nil
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return staticOllamaEmbedModels(), nil
	}

	// Filter to known embedding model families
	embedKeywords := []string{"embed", "nomic", "mxbai", "bge", "e5", "gte", "minilm", "snowflake"}
	var models []embeddings.ModelInfo
	for _, m := range result.Models {
		lower := strings.ToLower(m.Name)
		for _, kw := range embedKeywords {
			if strings.Contains(lower, kw) {
				models = append(models, embeddings.ModelInfo{
					ID: m.Name, Name: m.Name, Provider: "ollama-embed",
					Dimensions: 768, MaxInputTokens: 8192, Local: true,
				})
				break
			}
		}
	}
	if len(models) == 0 {
		return staticOllamaEmbedModels(), nil
	}
	return models, nil
}

func staticOllamaEmbedModels() []embeddings.ModelInfo {
	return []embeddings.ModelInfo{
		{ID: "nomic-embed-text", Name: "Nomic Embed Text", Provider: "ollama-embed", Dimensions: 768, MaxInputTokens: 8192, Local: true},
		{ID: "mxbai-embed-large", Name: "MxBAI Embed Large", Provider: "ollama-embed", Dimensions: 1024, MaxInputTokens: 512, Local: true},
		{ID: "bge-m3", Name: "BGE M3", Provider: "ollama-embed", Dimensions: 1024, MaxInputTokens: 8192, Local: true},
		{ID: "snowflake-arctic-embed", Name: "Snowflake Arctic Embed", Provider: "ollama-embed", Dimensions: 1024, MaxInputTokens: 512, Local: true},
	}
}

func (e *ollamaEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}
	vecs := make([][]float32, 0, len(req.Texts))
	for _, text := range req.Texts {
		body, _ := json.Marshal(map[string]any{"model": model, "input": text})
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := e.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		var result struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if len(result.Embeddings) > 0 {
			vecs = append(vecs, result.Embeddings[0])
		}
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	return &embeddings.EmbedResponse{Embeddings: vecs, Dimensions: dim, Model: model}, nil
}

// ─────────────────────────────────────────────
// Cohere Embeddings
// ─────────────────────────────────────────────

type cohereEmbedder struct {
	apiKey string
	client *http.Client
}

// NewCohere creates a Cohere embedding provider.
func NewCohere(apiKey string) embeddings.Embedder {
	return &cohereEmbedder{apiKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

func (e *cohereEmbedder) Name() string                        { return "cohere-embed" }
func (e *cohereEmbedder) DefaultModel() string                { return "embed-v4.0" }
func (e *cohereEmbedder) HealthCheck(_ context.Context) error { return nil }

func (e *cohereEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: "embed-v4.0", Name: "Embed v4.0", Provider: "cohere-embed", Dimensions: 1024, MaxInputTokens: 512, CostPerMTokens: 0.1},
		{ID: "embed-multilingual-v3.0", Name: "Embed Multilingual v3", Provider: "cohere-embed", Dimensions: 1024, MaxInputTokens: 512, CostPerMTokens: 0.1},
		{ID: "embed-english-v3.0", Name: "Embed English v3.0", Provider: "cohere-embed", Dimensions: 1024, MaxInputTokens: 512, CostPerMTokens: 0.1},
	}, nil
}

func (e *cohereEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}
	inputType := "search_document"
	if req.InputType == embeddings.InputTypeQuery {
		inputType = "search_query"
	}
	body, _ := json.Marshal(map[string]any{
		"model":           model,
		"texts":           req.Texts,
		"input_type":      inputType,
		"embedding_types": []string{"float"},
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.cohere.com/v2/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cohere embeddings: http %d", resp.StatusCode)
	}

	var result struct {
		Embeddings struct {
			Float [][]float32 `json:"float"`
		} `json:"embeddings"`
		Texts []string `json:"texts"`
		Meta  struct {
			BilledUnits struct {
				InputTokens int `json:"input_tokens"`
			} `json:"billed_units"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	dim := 0
	if len(result.Embeddings.Float) > 0 {
		dim = len(result.Embeddings.Float[0])
	}
	return &embeddings.EmbedResponse{
		Embeddings: result.Embeddings.Float,
		Dimensions: dim,
		Model:      model,
		TokensUsed: result.Meta.BilledUnits.InputTokens,
	}, nil
}

// ─────────────────────────────────────────────
// Google Embeddings
// ─────────────────────────────────────────────

type googleEmbedder struct {
	apiKey string
	client *http.Client
}

// NewGoogle creates a Google Generative AI embedding provider.
func NewGoogle(apiKey string) embeddings.Embedder {
	return &googleEmbedder{apiKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

func (e *googleEmbedder) Name() string                        { return "google-embed" }
func (e *googleEmbedder) DefaultModel() string                { return "text-embedding-004" }
func (e *googleEmbedder) HealthCheck(_ context.Context) error { return nil }

func (e *googleEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: "text-embedding-004", Name: "Text Embedding 004", Provider: "google-embed", Dimensions: 768, MaxInputTokens: 2048, CostPerMTokens: 0.0},
		{ID: "gemini-embedding-exp-03-07", Name: "Gemini Embedding Exp", Provider: "google-embed", Dimensions: 3072, MaxInputTokens: 8192, CostPerMTokens: 0.0},
	}, nil
}

func (e *googleEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}

	// Google uses batch embed API
	type textInput struct {
		TaskType string `json:"taskType,omitempty"`
		Content  struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	}
	var inputs []textInput
	taskType := "RETRIEVAL_DOCUMENT"
	if req.InputType == embeddings.InputTypeQuery {
		taskType = "RETRIEVAL_QUERY"
	}
	for _, text := range req.Texts {
		ti := textInput{TaskType: taskType}
		ti.Content.Parts = []struct {
			Text string `json:"text"`
		}{{Text: text}}
		inputs = append(inputs, ti)
	}

	body, _ := json.Marshal(map[string]any{"requests": inputs})
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s", model, e.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google embeddings: http %d", resp.StatusCode)
	}

	var result struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vecs[i] = emb.Values
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	return &embeddings.EmbedResponse{Embeddings: vecs, Dimensions: dim, Model: model}, nil
}

// ─────────────────────────────────────────────
// HuggingFace Embeddings
// ─────────────────────────────────────────────

type huggingfaceEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewHuggingFace creates a HuggingFace Inference API embedding provider.
func NewHuggingFace(apiKey, baseURL, model string) embeddings.Embedder {
	if baseURL == "" {
		baseURL = "https://api-inference.huggingface.co"
	}
	if model == "" {
		model = "sentence-transformers/all-MiniLM-L6-v2"
	}
	return &huggingfaceEmbedder{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: model, client: &http.Client{Timeout: 60 * time.Second}}
}

func (e *huggingfaceEmbedder) Name() string                        { return "huggingface-embed" }
func (e *huggingfaceEmbedder) DefaultModel() string                { return e.model }
func (e *huggingfaceEmbedder) HealthCheck(_ context.Context) error { return nil }

func (e *huggingfaceEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: e.model, Name: e.model, Provider: "huggingface-embed", Dimensions: 384},
	}, nil
}

func (e *huggingfaceEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.model
	}
	body, _ := json.Marshal(map[string]any{"inputs": req.Texts})
	url := fmt.Sprintf("%s/models/%s", e.baseURL, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var vecs [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&vecs); err != nil {
		return nil, err
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	return &embeddings.EmbedResponse{Embeddings: vecs, Dimensions: dim, Model: model}, nil
}

// ─────────────────────────────────────────────
// Azure OpenAI Embeddings
// ─────────────────────────────────────────────

type azureEmbedder struct {
	apiKey     string
	baseURL    string // https://{resource}.openai.azure.com
	apiVersion string
	client     *http.Client
}

// NewAzure creates an Azure OpenAI embedding provider.
func NewAzure(apiKey, baseURL, apiVersion string) embeddings.Embedder {
	if apiVersion == "" {
		apiVersion = "2024-02-15-preview"
	}
	return &azureEmbedder{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiVersion: apiVersion,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *azureEmbedder) Name() string                        { return "azure" }
func (e *azureEmbedder) DefaultModel() string                { return "text-embedding-3-small" }
func (e *azureEmbedder) HealthCheck(_ context.Context) error { return nil }

func (e *azureEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Provider: "azure", Dimensions: 1536},
		{ID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Provider: "azure", Dimensions: 3072},
		{ID: "text-embedding-ada-002", Name: "Text Embedding Ada 002", Provider: "azure", Dimensions: 1536},
	}, nil
}

func (e *azureEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}
	body, _ := json.Marshal(map[string]any{"input": req.Texts})
	// Azure URL format: {endpoint}/openai/deployments/{deployment-id}/embeddings?api-version={api-version}
	url := fmt.Sprintf("%s/openai/deployments/%s/embeddings?api-version=%s", e.baseURL, model, e.apiVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", e.apiKey)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure embeddings: http %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		vecs[d.Index] = d.Embedding
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	return &embeddings.EmbedResponse{
		Embeddings: vecs,
		Dimensions: dim,
		Model:      model,
		TokensUsed: result.Usage.TotalTokens,
	}, nil
}

// ─────────────────────────────────────────────
// Bedrock Embeddings
// ─────────────────────────────────────────────

// NOTE: Bedrock embeddings are handled via the AWS SDK in a separate file or
// via raw HTTP if we want to avoid complex dependency cycles here.
// Given Bedrock's complex SigV4 signing, using the SDK is much better.
// I will add a placeholder here and implement it properly if I can import bedrockruntime.
// For now, let's assume we implement it in a way that avoids circular imports.
