package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// FailoverProvider wraps an ordered list of primary providers with circuit-breaker
// logic and an optional local fallback (e.g. gemma). It implements provider.Provider.
type FailoverProvider struct {
	primary  []Provider
	fallback Provider
	breakers map[string]*circuitBreaker
	log      *zap.Logger
}

// circuitBreaker tracks failure state for a single provider.
type circuitBreaker struct {
	mu            sync.RWMutex
	failures      int
	openUntil     time.Time
	threshold     int
	cooldown      time.Duration
	halfOpenProbe bool
}

// NewFailoverProvider creates a failover wrapper around the given providers.
// The fallback may be nil if no local model is configured.
func NewFailoverProvider(primary []Provider, fallback Provider, log *zap.Logger) *FailoverProvider {
	if log == nil {
		log = zap.NewNop()
	}
	breakers := make(map[string]*circuitBreaker, len(primary))
	for _, p := range primary {
		breakers[p.Name()] = &circuitBreaker{
			threshold: 3,
			cooldown:  60 * time.Second,
		}
	}
	return &FailoverProvider{
		primary:  primary,
		fallback: fallback,
		breakers: breakers,
		log:      log,
	}
}

// Name returns "failover".
func (f *FailoverProvider) Name() string { return "failover" }

// Mode returns "online", "degraded", or "offline" based on breaker states.
func (f *FailoverProvider) Mode() string {
	if f.fallback != nil {
		if err := f.fallback.HealthCheck(context.Background()); err != nil {
			// Fallback also dead → offline.
			allOpen := true
			for _, p := range f.primary {
				if !f.isOpen(p.Name()) {
					allOpen = false
					break
				}
			}
			if allOpen {
				return "offline"
			}
		}
	}
	// At least one primary breaker is open but fallback is healthy → degraded.
	for _, p := range f.primary {
		if f.isOpen(p.Name()) {
			return "degraded"
		}
	}
	return "online"
}

// Complete tries each primary provider in order, then the fallback.
func (f *FailoverProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	return f.tryComplete(ctx, req)
}

// Stream tries each primary provider in order, then the fallback.
func (f *FailoverProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	return f.tryStream(ctx, req)
}

// ListModels delegates to the first healthy primary. Falls back if needed.
func (f *FailoverProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return f.tryModels(ctx)
}

// HealthCheck succeeds if any primary (or fallback) is healthy.
func (f *FailoverProvider) HealthCheck(ctx context.Context) error {
	for _, p := range f.available() {
		if err := p.HealthCheck(ctx); err == nil {
			return nil
		}
	}
	if f.fallback != nil {
		return f.fallback.HealthCheck(ctx)
	}
	return fmt.Errorf("no healthy provider")
}

// Embed delegates to the first healthy primary. Falls back if needed.
func (f *FailoverProvider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return f.tryEmbed(ctx, model, text)
}

// ValidateModel delegates to the first healthy primary.
func (f *FailoverProvider) ValidateModel(ctx context.Context, modelID string) error {
	for _, p := range f.available() {
		if err := p.ValidateModel(ctx, modelID); err == nil {
			return nil
		}
	}
	if f.fallback != nil {
		return f.fallback.ValidateModel(ctx, modelID)
	}
	return fmt.Errorf("no provider validates model %q", modelID)
}

// Caps returns the caps of the first available primary, or a conservative default.
func (f *FailoverProvider) Caps() ProviderCaps {
	for _, p := range f.available() {
		return p.Caps()
	}
	if f.fallback != nil {
		return f.fallback.Caps()
	}
	return ProviderCaps{}
}

// tryComplete iterates over available primaries, then fallback, for Complete.
func (f *FailoverProvider) tryComplete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	var lastErr error
	for _, p := range f.available() {
		res, err := p.Complete(ctx, req)
		if err == nil {
			f.resetBreaker(p.Name())
			return res, nil
		}
		lastErr = err
		if !isTransient(err) {
			f.log.Warn("non-transient provider error, aborting failover", zap.String("provider", p.Name()), zap.Error(err))
			return nil, err
		}
		f.recordFailure(p.Name())
		f.log.Warn("provider transient failure, trying next", zap.String("provider", p.Name()), zap.Error(err))
	}
	if f.fallback != nil {
		f.log.Info("all primary providers failed, trying fallback", zap.String("fallback", f.fallback.Name()))
		res, err := f.fallback.Complete(ctx, req)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// tryStream iterates over available primaries, then fallback, for Stream.
func (f *FailoverProvider) tryStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	var lastErr error
	for _, p := range f.available() {
		res, err := p.Stream(ctx, req)
		if err == nil {
			f.resetBreaker(p.Name())
			return res, nil
		}
		lastErr = err
		if !isTransient(err) {
			f.log.Warn("non-transient provider error, aborting failover", zap.String("provider", p.Name()), zap.Error(err))
			return nil, err
		}
		f.recordFailure(p.Name())
		f.log.Warn("provider transient failure, trying next", zap.String("provider", p.Name()), zap.Error(err))
	}
	if f.fallback != nil {
		f.log.Info("all primary providers failed, trying fallback", zap.String("fallback", f.fallback.Name()))
		res, err := f.fallback.Stream(ctx, req)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// tryModels iterates over available primaries, then fallback, for ListModels.
func (f *FailoverProvider) tryModels(ctx context.Context) ([]ModelInfo, error) {
	var lastErr error
	for _, p := range f.available() {
		res, err := p.ListModels(ctx)
		if err == nil {
			f.resetBreaker(p.Name())
			return res, nil
		}
		lastErr = err
		if !isTransient(err) {
			f.log.Warn("non-transient provider error, aborting failover", zap.String("provider", p.Name()), zap.Error(err))
			return nil, err
		}
		f.recordFailure(p.Name())
		f.log.Warn("provider transient failure, trying next", zap.String("provider", p.Name()), zap.Error(err))
	}
	if f.fallback != nil {
		res, err := f.fallback.ListModels(ctx)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// tryEmbed iterates over available primaries, then fallback, for Embed.
func (f *FailoverProvider) tryEmbed(ctx context.Context, model string, text string) ([]float32, error) {
	var lastErr error
	for _, p := range f.available() {
		res, err := p.Embed(ctx, model, text)
		if err == nil {
			f.resetBreaker(p.Name())
			return res, nil
		}
		lastErr = err
		if !isTransient(err) {
			f.log.Warn("non-transient provider error, aborting failover", zap.String("provider", p.Name()), zap.Error(err))
			return nil, err
		}
		f.recordFailure(p.Name())
		f.log.Warn("provider transient failure, trying next", zap.String("provider", p.Name()), zap.Error(err))
	}
	if f.fallback != nil {
		res, err := f.fallback.Embed(ctx, model, text)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// available returns primaries whose breakers are not currently open.
func (f *FailoverProvider) available() []Provider {
	var out []Provider
	for _, p := range f.primary {
		if !f.isOpen(p.Name()) {
			out = append(out, p)
		}
	}
	return out
}

func (f *FailoverProvider) isOpen(name string) bool {
	b, ok := f.breakers[name]
	if !ok {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if time.Now().After(b.openUntil) {
		return false
	}
	return true
}

func (f *FailoverProvider) recordFailure(name string) {
	b, ok := f.breakers[name]
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = time.Now().Add(b.cooldown)
		b.halfOpenProbe = true
		f.log.Warn("provider breaker opened",
			zap.String("provider", name),
			zap.Int("failures", b.failures),
			zap.Duration("cooldown", b.cooldown),
		)
	}
}

func (f *FailoverProvider) resetBreaker(name string) {
	b, ok := f.breakers[name]
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures > 0 {
		b.failures = 0
		b.halfOpenProbe = false
		f.log.Info("provider breaker reset", zap.String("provider", name))
	}
}

// isTransient reports whether an error justifies trying the next provider.
func isTransient(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true // timeout → try next provider
	}

	var pErr *ProviderError
	if errors.As(err, &pErr) {
		return pErr.Retryable
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}
