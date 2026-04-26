// Package plano implements an AssistClaw provider backed by Plano,
// an open-source AI proxy that automatically routes prompts to the
// right model based on complexity (https://github.com/katanemo/plano).
//
// Plano exposes an OpenAI-compatible /v1/chat/completions endpoint,
// so this provider is a thin wrapper around openaicompat.Provider.
package plano

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/provider/openaicompat"
)

const (
	ProviderName    = "plano"
	DefaultEndpoint = "http://localhost:12000/v1"
	DefaultTimeout  = 120 * time.Second
)

// Config holds Plano-specific settings from assistclaw.yaml.
type Config struct {
	Enabled          bool         `yaml:"enabled"`
	Endpoint         string       `yaml:"endpoint"`          // default: http://localhost:12000/v1
	FallbackProvider string       `yaml:"fallback_provider"` // e.g. "openai" — used if Plano unreachable
	Preferences      []Preference `yaml:"preferences"`
}

// Preference maps a plain-English description to a preferred model.
type Preference struct {
	Description string `yaml:"description"`
	PreferModel string `yaml:"prefer_model"`
}

// Provider wraps openaicompat.Provider and adds Plano-specific behaviour.
type Provider struct {
	inner    *openaicompat.Provider
	cfg      Config
	fallback provider.Provider // optional fallback
}

// New creates a Plano provider.  If endpoint is empty, DefaultEndpoint is used.
func New(cfg Config, fallback provider.Provider) *Provider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	inner := openaicompat.New(openaicompat.Config{
		Name:    ProviderName,
		BaseURL: cfg.Endpoint,
		// No API key — Plano is local
		DiscoverModels: true,
		StaticModels:   planoStaticModels(cfg),
	})
	return &Provider{inner: inner, cfg: cfg, fallback: fallback}
}

// Name returns "plano".
func (p *Provider) Name() string { return ProviderName }

func (p *Provider) Caps() provider.ProviderCaps { return p.inner.Caps() }

// HealthCheck verifies Plano is reachable.
func (p *Provider) HealthCheck(ctx context.Context) error {
	return p.ValidateModel(ctx, "")
}

func (p *Provider) ValidateModel(ctx context.Context, modelID string) error {
	// Plano usually handles routing, so if it's reachable, it's "healthy".
	// But we can check its /models endpoint.
	cl := &http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Get(p.cfg.Endpoint + "/models")
	if err != nil {
		if p.fallback != nil {
			return p.fallback.ValidateModel(ctx, modelID)
		}
		return fmt.Errorf("plano unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("plano health check failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// ListModels proxies to inner with the static model list from preferences.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return p.inner.ListModels(ctx)
}

// Embed proxies embedding requests to the inner provider (or fallback).
func (p *Provider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	vec, err := p.inner.Embed(ctx, model, text)
	if err != nil && p.fallback != nil {
		return p.fallback.Embed(ctx, model, text)
	}
	return vec, err
}

// Complete routes through Plano; falls back to the fallback provider on error.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	resp, err := p.inner.Complete(ctx, req)
	if err != nil && p.fallback != nil {
		return p.fallback.Complete(ctx, req)
	}
	return resp, err
}

// Stream routes through Plano with fallback support.
func (p *Provider) Stream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	ch, err := p.inner.Stream(ctx, req)
	if err != nil && p.fallback != nil {
		return p.fallback.Stream(ctx, req)
	}
	return ch, err
}

func (p *Provider) SupportsNativeStreaming() bool { return p.inner.SupportsNativeStreaming() }

// planoStaticModels builds a model list from routing preferences so that
// ListModels returns something sensible even before Plano is running.
func planoStaticModels(cfg Config) []provider.ModelInfo {
	seen := make(map[string]bool)
	var models []provider.ModelInfo
	for _, pref := range cfg.Preferences {
		if pref.PreferModel != "" && !seen[pref.PreferModel] {
			seen[pref.PreferModel] = true
			models = append(models, provider.ModelInfo{
				ID:   pref.PreferModel,
				Name: pref.PreferModel,
			})
		}
	}
	if len(models) == 0 {
		models = append(models, provider.ModelInfo{ID: "plano-router", Name: "Plano Router"})
	}
	return models
}
