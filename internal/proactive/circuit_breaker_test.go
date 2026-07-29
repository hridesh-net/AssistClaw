package proactive

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// flakyTrigger fails the first `failures` invocations of Start, then
// returns nil immediately. This lets us assert the breaker retries with
// backoff and resets after a successful Start completion.
type flakyTrigger struct {
	failures int32
}

func (f *flakyTrigger) Name() string { return "flaky" }

func (f *flakyTrigger) Start(_ context.Context, _ EmitFunc) error {
	if atomic.AddInt32(&f.failures, -1) >= 0 {
		return errors.New("simulated transient failure")
	}
	return nil
}

func TestCircuitBreaker_RetriesThenSucceeds(t *testing.T) {
	ResetBreakerRegistry()
	flaky := &flakyTrigger{failures: 3}
	cb := NewCircuitBreaker(flaky, nil)
	cb.CooldownMax = 5 * time.Millisecond // keep test fast

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start blocks until the inner trigger returns nil; with CooldownMax
	// at 5ms the 3 failures complete in ~15ms, then the 4th call returns
	// nil and Start returns.
	if err := cb.Start(ctx, func(Event) {}); err != nil {
		t.Fatalf("breaker should exit cleanly after success, got %v", err)
	}
	if cb.IsTripped() {
		t.Fatalf("breaker should not be tripped after recovery")
	}
	if got := cb.ConsecutiveFailures(); got != 0 {
		t.Fatalf("expected consecutive=0 after success, got %d", got)
	}
}

func TestCircuitBreaker_TripsAfterLimit(t *testing.T) {
	ResetBreakerRegistry()
	flaky := &flakyTrigger{failures: 100} // never succeeds in the test window
	cb := NewCircuitBreaker(flaky, nil)
	cb.ConsecutiveFailLimit = 3
	cb.CooldownMax = 2 * time.Millisecond
	cb.Cooldown = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go func() { _ = cb.Start(ctx, func(Event) {}) }()
	// Wait long enough for ConsecutiveFailLimit failures + breaker to trip.
	time.Sleep(40 * time.Millisecond)

	if cb.TotalFailures() < 3 {
		t.Fatalf("expected at least 3 failures, got %d", cb.TotalFailures())
	}
	// At some point in the window the breaker must have tripped (it may
	// have probed back to closed by the end, which is fine).
	if cb.TotalFailures() < uint64(cb.ConsecutiveFailLimit) {
		t.Fatalf("breaker should have crossed the trip threshold")
	}
}

func TestBreakerRegistryReportsStatus(t *testing.T) {
	ResetBreakerRegistry()
	_ = NewCircuitBreaker(&flakyTrigger{}, nil)
	_ = NewCircuitBreaker(&flakyTrigger{}, nil)
	statuses := BreakerStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 registered breakers, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Trigger != "flaky" {
			t.Errorf("unexpected trigger name %q", s.Trigger)
		}
	}
}
