// Package provider: registry — discovers, registers, and routes to LLM providers.
package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ─────────────────────────────────────────────
// Registry
// ─────────────────────────────────────────────

// Registry holds all registered providers and their model catalogs.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	catalog   map[string]ModelInfo // key: "provider/model-id"
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		catalog:   make(map[string]ModelInfo),
	}
}

// Register adds a provider to the registry and discovers its models.
// It is safe to call Register concurrently.
func (r *Registry) Register(ctx context.Context, p Provider) error {
	r.mu.Lock()
	r.providers[p.Name()] = p
	r.mu.Unlock()

	models, err := p.ListModels(ctx)
	if err != nil {
		// Non-fatal: provider is registered but model list is empty.
		// This allows connecting to unreachable local providers without
		// blocking startup.
		return fmt.Errorf("register %s: list models: %w", p.Name(), err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range models {
		key := r.catalogKey(m.Provider, m.ID)
		r.catalog[key] = m
	}
	return nil
}

// Get returns the provider registered under name.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// All returns all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	return out
}

// ListModels returns every model across all providers.
func (r *Registry) ListModels() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelInfo, 0, len(r.catalog))
	for _, m := range r.catalog {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ResolveModel parses a model string in "provider/model-id" format and returns
// the matching provider and model info. If only a model ID is provided (no "/"),
// it searches all providers for a matching model.
func (r *Registry) ResolveModel(ctx context.Context, modelStr string) (Provider, ModelInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parts := strings.SplitN(modelStr, "/", 2)
	if len(parts) == 2 {
		providerName := parts[0]
		modelID := parts[1]
		p, ok := r.providers[providerName]
		if !ok {
			return nil, ModelInfo{}, fmt.Errorf("provider %q not found", providerName)
		}
		key := r.catalogKey(providerName, modelID)
		info, ok := r.catalog[key]
		if !ok {
			// Model may be valid but not in catalog (dynamic provider); create
			// a minimal ModelInfo AFTER validating with the provider.
			if err := p.ValidateModel(ctx, modelID); err != nil {
				return nil, ModelInfo{}, err
			}
			info = ModelInfo{ID: modelID, Provider: providerName}
		}
		return p, info, nil
	}

	// Search all providers for matching model by ID (stable order so duplicates resolve predictably).
	modelID := parts[0]
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		p := r.providers[name]
		key := r.catalogKey(p.Name(), modelID)
		if info, ok := r.catalog[key]; ok {
			return p, info, nil
		}
	}
	return nil, ModelInfo{}, fmt.Errorf("model %q not found in any registered provider", modelID)
}

func (r *Registry) catalogKey(providerName, modelID string) string {
	return providerName + "/" + modelID
}

// ─────────────────────────────────────────────
// Health check helpers
// ─────────────────────────────────────────────

// HealthReport summarises health for all registered providers.
type HealthReport struct {
	Results map[string]HealthResult
}

// HealthResult is a single provider health check result.
type HealthResult struct {
	OK    bool
	Error string
}

// CheckAll runs HealthCheck on all registered providers concurrently.
func (r *Registry) CheckAll(ctx context.Context) HealthReport {
	r.mu.RLock()
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	r.mu.RUnlock()

	results := make(chan struct {
		name   string
		result HealthResult
	}, len(providers))

	for _, p := range providers {
		p := p
		go func() {
			err := p.HealthCheck(ctx)
			hr := HealthResult{OK: err == nil}
			if err != nil {
				hr.Error = err.Error()
			}
			results <- struct {
				name   string
				result HealthResult
			}{p.Name(), hr}
		}()
	}

	report := HealthReport{Results: make(map[string]HealthResult)}
	for range providers {
		r2 := <-results
		report.Results[r2.name] = r2.result
	}
	return report
}
