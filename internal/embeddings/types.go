// Package embeddings defines the unified embedding model interface and shared types.
// All embedding providers implement the Embedder interface.
package embeddings

import (
	"context"
	"fmt"
	"sync"
)

// ─────────────────────────────────────────────
// Core types
// ─────────────────────────────────────────────

// EmbedRequest is a batch request to embed one or more texts.
type EmbedRequest struct {
	Model string   `json:"model"`
	Texts []string `json:"texts"`
	// InputType hints to the model how the text will be used (query vs document).
	// Some providers (Cohere) use this for asymmetric embeddings.
	InputType InputType `json:"input_type,omitempty"`
}

// InputType classifies the usage of embedded text.
type InputType string

const (
	InputTypeQuery    InputType = "query"
	InputTypeDocument InputType = "document"
)

// EmbedResponse holds vectors for each input text.
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	// Dimensions is the length of each embedding vector.
	Dimensions int `json:"dimensions"`
	// Model reports the model that was actually used (after alias resolution).
	Model string `json:"model"`
	// TokensUsed is the total number of tokens processed (where available).
	TokensUsed int `json:"tokens_used,omitempty"`
}

// ModelInfo describes an embedding model.
type ModelInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Dimensions int    `json:"dimensions"`
	// MaxInputTokens is the maximum number of tokens per input string.
	MaxInputTokens int  `json:"max_input_tokens"`
	Local          bool `json:"local"`
	// CostPerMTokens is the USD cost per million input tokens (0 for local).
	CostPerMTokens float64 `json:"cost_per_m_tokens,omitempty"`
}

// ─────────────────────────────────────────────
// Embedder interface
// ─────────────────────────────────────────────

// Embedder is the unified interface every embedding backend must implement.
type Embedder interface {
	// Name returns the canonical provider identifier.
	Name() string

	// Embed generates embeddings for a batch of texts.
	// Returns one vector per input text, in the same order.
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)

	// ListModels returns all embedding models available from this provider.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// DefaultModel returns the model ID used when none is specified.
	DefaultModel() string

	// HealthCheck verifies the provider is reachable and credentials are valid.
	HealthCheck(ctx context.Context) error
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Both vectors must have the same dimension.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}
	normA = sqrt32(normA)
	normB = sqrt32(normB)
	return dot / (normA * normB), nil
}

// sqrt32 is a float32 square root (avoiding cgo/math import in this file).
func sqrt32(x float32) float32 {
	if x < 0 {
		return 0
	}
	z := float32(1.0)
	for i := 0; i < 15; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}

// ─────────────────────────────────────────────
// Registry
// ─────────────────────────────────────────────

// Registry manages all registered embedding providers.
type Registry struct {
	mu        sync.RWMutex
	embedders map[string]Embedder
	catalog   map[string]ModelInfo // key: "provider/model-id"
	// priority is the ordered list of provider names to try when no explicit
	// provider is requested.
	priority []string
}

// NewRegistry creates an empty embedding registry.
func NewRegistry() *Registry {
	return &Registry{
		embedders: make(map[string]Embedder),
		catalog:   make(map[string]ModelInfo),
	}
}

// Register adds an embedder and discovers its models.
func (r *Registry) Register(ctx context.Context, e Embedder) error {
	r.mu.Lock()
	r.embedders[e.Name()] = e
	r.priority = append(r.priority, e.Name())
	r.mu.Unlock()

	models, err := e.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("register embedder %s: list models: %w", e.Name(), err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range models {
		key := m.Provider + "/" + m.ID
		r.catalog[key] = m
	}
	return nil
}

// Get returns the embedder registered under name.
func (r *Registry) Get(name string) (Embedder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.embedders[name]
	return e, ok
}

// Default returns the first available embedder in priority order.
func (r *Registry) Default() (Embedder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range r.priority {
		if e, ok := r.embedders[name]; ok {
			return e, true
		}
	}
	return nil, false
}

// EmbedText is a convenience wrapper that embeds a single text using the
// default embedder.
func (r *Registry) EmbedText(ctx context.Context, text string) ([]float32, error) {
	e, ok := r.Default()
	if !ok {
		return nil, fmt.Errorf("no embedding provider registered")
	}
	resp, err := e.Embed(ctx, &EmbedRequest{
		Model:     e.DefaultModel(),
		Texts:     []string{text},
		InputType: InputTypeDocument,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("embedding provider returned no vectors")
	}
	return resp.Embeddings[0], nil
}

// EmbedQuery embeds a query string using the default embedder.
func (r *Registry) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	e, ok := r.Default()
	if !ok {
		return nil, fmt.Errorf("no embedding provider registered")
	}
	resp, err := e.Embed(ctx, &EmbedRequest{
		Model:     e.DefaultModel(),
		Texts:     []string{query},
		InputType: InputTypeQuery,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("embedding provider returned no vectors")
	}
	return resp.Embeddings[0], nil
}

// ListModels lists all embedding models across all providers.
func (r *Registry) ListModels() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelInfo, 0, len(r.catalog))
	for _, m := range r.catalog {
		out = append(out, m)
	}
	return out
}
