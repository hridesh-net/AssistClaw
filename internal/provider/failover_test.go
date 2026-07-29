package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeProvider is a test double that returns configured results.
type fakeProvider struct {
	name       string
	completeFn func(context.Context, *CompletionRequest) (*CompletionResponse, error)
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if f.completeFn != nil {
		return f.completeFn(ctx, req)
	}
	return nil, errors.New("no complete fn")
}

func (f *fakeProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	return nil, errors.New("stream not implemented")
}

func (f *fakeProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return nil, errors.New("list models not implemented")
}

func (f *fakeProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (f *fakeProvider) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return nil, errors.New("embed not implemented")
}

func (f *fakeProvider) ValidateModel(ctx context.Context, modelID string) error {
	return nil
}

func (f *fakeProvider) Caps() ProviderCaps {
	return ProviderCaps{}
}

func TestFailoverProvider_Complete_firstSucceeds(t *testing.T) {
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p1"}}}, nil
	}}
	p2 := &fakeProvider{name: "p2", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p2"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1, p2}, nil, zap.NewNop())
	res, err := fp.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text() != "p1" {
		t.Fatalf("expected p1, got %s", res.Text())
	}
}

func TestFailoverProvider_Complete_triesNextOnTransient(t *testing.T) {
	calls := 0
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		calls++
		return nil, &ProviderError{Provider: "p1", StatusCode: 503, Retryable: true}
	}}
	p2 := &fakeProvider{name: "p2", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		calls++
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p2"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1, p2}, nil, zap.NewNop())
	res, err := fp.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text() != "p2" {
		t.Fatalf("expected p2, got %s", res.Text())
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestFailoverProvider_Complete_abortsOnNonTransient(t *testing.T) {
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return nil, &ProviderError{Provider: "p1", StatusCode: 401, Retryable: false}
	}}
	p2 := &fakeProvider{name: "p2", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p2"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1, p2}, nil, zap.NewNop())
	_, err := fp.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	// p2 should NOT be called because 401 is non-transient.
	var pErr *ProviderError
	if !errors.As(err, &pErr) || pErr.StatusCode != 401 {
		t.Fatalf("expected 401 ProviderError, got %v", err)
	}
}

func TestFailoverProvider_Complete_usesFallback(t *testing.T) {
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return nil, &ProviderError{Provider: "p1", StatusCode: 503, Retryable: true}
	}}
	fallback := &fakeProvider{name: "gemma", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "gemma"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1}, fallback, zap.NewNop())
	res, err := fp.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text() != "gemma" {
		t.Fatalf("expected gemma, got %s", res.Text())
	}
}

func TestFailoverProvider_CircuitBreaker_opensAfterThreshold(t *testing.T) {
	calls := 0
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		calls++
		return nil, &ProviderError{Provider: "p1", StatusCode: 503, Retryable: true}
	}}
	p2 := &fakeProvider{name: "p2", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p2"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1, p2}, nil, zap.NewNop())
	// Override breaker threshold for faster test.
	fp.breakers["p1"].threshold = 2

	// First call: p1 fails, p2 succeeds.
	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	if calls != 1 {
		t.Fatalf("expected 1 call after first request, got %d", calls)
	}

	// Second call: p1 fails again, breaker should open.
	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	if calls != 2 {
		t.Fatalf("expected 2 calls after second request, got %d", calls)
	}

	// Third call: p1 breaker is open, so p1 should be skipped.
	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	if calls != 2 {
		t.Fatalf("expected p1 skipped (breaker open), got %d calls", calls)
	}
}

func TestFailoverProvider_CircuitBreaker_resetsOnSuccess(t *testing.T) {
	calls := 0
	p1 := &fakeProvider{name: "p1", completeFn: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
		calls++
		if calls <= 2 {
			return nil, &ProviderError{Provider: "p1", StatusCode: 503, Retryable: true}
		}
		return &CompletionResponse{Content: []ContentPart{{Type: ContentTypeText, Text: "p1"}}}, nil
	}}

	fp := NewFailoverProvider([]Provider{p1}, nil, zap.NewNop())
	fp.breakers["p1"].threshold = 3

	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}

	// Force breaker open by one more failure.
	_, _ = fp.Complete(context.Background(), &CompletionRequest{})
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}

	// Wait for cooldown.
	fp.breakers["p1"].cooldown = 1 * time.Millisecond
	time.Sleep(5 * time.Millisecond)

	// Next call should try p1 again (breaker closed) and succeed.
	res, err := fp.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text() != "p1" {
		t.Fatalf("expected p1, got %s", res.Text())
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"retryable provider", &ProviderError{Retryable: true}, true},
		{"non-retryable provider", &ProviderError{Retryable: false}, false},
		{"generic error", fmt.Errorf("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransient(tt.err); got != tt.want {
				t.Fatalf("isTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
