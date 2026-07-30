package proactive

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// CircuitBreakerTrigger wraps another Trigger with restart, exponential
// backoff, and a circuit breaker so a flapping source (e.g. an IMAP IDLE
// connection that keeps dropping) does not silently die after the first
// error nor pin the daemon at 100% CPU reconnecting in a tight loop.
//
// Behaviour:
//   - inner.Start is run in a goroutine.
//   - If it returns nil, the breaker exits successfully.
//   - If it returns an error, retry with exponential backoff capped at
//     CooldownMax (default 5m), counting consecutive failures.
//   - After ConsecutiveFailLimit consecutive failures the breaker is
//     "tripped": it sleeps for Cooldown (default 10m) before probing
//     again with one attempt.
//   - Context cancellation always exits cleanly.
type CircuitBreakerTrigger struct {
	Inner Trigger
	Log   *zap.Logger

	// ConsecutiveFailLimit defaults to 5.
	ConsecutiveFailLimit int
	// Cooldown is the sleep after the breaker trips (default 10m).
	Cooldown time.Duration
	// CooldownMax is the cap on per-retry backoff (default 5m).
	CooldownMax time.Duration

	// Atomic counters surface state for /doctor and /status.
	consecutive int32
	tripped     int32 // 0 = closed, 1 = open
	totalFails  uint64
}

// NewCircuitBreaker wraps inner with sensible defaults and registers it
// in the package-wide registry so health checks can enumerate state.
func NewCircuitBreaker(inner Trigger, log *zap.Logger) *CircuitBreakerTrigger {
	cb := &CircuitBreakerTrigger{
		Inner:                inner,
		Log:                  log,
		ConsecutiveFailLimit: 5,
		Cooldown:             10 * time.Minute,
		CooldownMax:          5 * time.Minute,
	}
	if cb.Log == nil {
		cb.Log = zap.NewNop()
	}
	registerBreaker(cb)
	return cb
}

// Name returns the inner trigger's name suffixed with "+cb".
func (c *CircuitBreakerTrigger) Name() string { return c.Inner.Name() + "+cb" }

// Start runs the inner trigger under the breaker policy until ctx ends.
func (c *CircuitBreakerTrigger) Start(ctx context.Context, emit EmitFunc) error {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := c.Inner.Start(ctx, emit)
		if err == nil {
			c.reset()
			return nil
		}
		// Context cancel is not a real failure.
		if ctx.Err() != nil {
			return err
		}

		atomic.AddInt32(&c.consecutive, 1)
		atomic.AddUint64(&c.totalFails, 1)
		attempt++

		if int(atomic.LoadInt32(&c.consecutive)) >= c.ConsecutiveFailLimit {
			atomic.StoreInt32(&c.tripped, 1)
			c.Log.Warn("circuit breaker tripped",
				zap.String("trigger", c.Inner.Name()),
				zap.Int("consecutive_failures", int(atomic.LoadInt32(&c.consecutive))),
				zap.Duration("cooldown", c.Cooldown),
				zap.Error(err),
			)
			if !sleepCtx(ctx, c.Cooldown) {
				return ctx.Err()
			}
			// Probe: reset the counter so a successful probe re-closes
			// the breaker. If the probe also fails we'll trip again.
			atomic.StoreInt32(&c.consecutive, 0)
			atomic.StoreInt32(&c.tripped, 0)
			continue
		}

		// Exponential backoff between retries: 1s, 2s, 4s, …, capped at CooldownMax.
		delay := time.Duration(1<<uint(attempt-1)) * time.Second
		if delay > c.CooldownMax {
			delay = c.CooldownMax
		}
		c.Log.Warn("trigger failed, will retry",
			zap.String("trigger", c.Inner.Name()),
			zap.Duration("retry_in", delay),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
		if !sleepCtx(ctx, delay) {
			return ctx.Err()
		}
	}
}

func (c *CircuitBreakerTrigger) reset() {
	atomic.StoreInt32(&c.consecutive, 0)
	atomic.StoreInt32(&c.tripped, 0)
}

// IsTripped reports whether the breaker is currently in the cooldown state.
func (c *CircuitBreakerTrigger) IsTripped() bool {
	return atomic.LoadInt32(&c.tripped) == 1
}

// ConsecutiveFailures returns the current streak of failures since the
// last success.
func (c *CircuitBreakerTrigger) ConsecutiveFailures() int {
	return int(atomic.LoadInt32(&c.consecutive))
}

// TotalFailures returns the lifetime failure count for this breaker.
func (c *CircuitBreakerTrigger) TotalFailures() uint64 {
	return atomic.LoadUint64(&c.totalFails)
}

// sleepCtx returns true if the sleep completed normally, false if ctx
// expired during the sleep.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// ─── Package-wide breaker registry ─────────────────────────────────────────

var (
	breakerMu sync.RWMutex
	breakers  []*CircuitBreakerTrigger
)

func registerBreaker(c *CircuitBreakerTrigger) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	breakers = append(breakers, c)
}

// BreakerStatus is a compact health snapshot used by /doctor and /status.
type BreakerStatus struct {
	Trigger     string
	Tripped     bool
	Consecutive int
	Total       uint64
}

// BreakerStatuses returns the current state of every wrapped trigger.
func BreakerStatuses() []BreakerStatus {
	breakerMu.RLock()
	defer breakerMu.RUnlock()
	out := make([]BreakerStatus, 0, len(breakers))
	for _, c := range breakers {
		out = append(out, BreakerStatus{
			Trigger:     c.Inner.Name(),
			Tripped:     c.IsTripped(),
			Consecutive: c.ConsecutiveFailures(),
			Total:       c.TotalFailures(),
		})
	}
	return out
}

// ResetBreakerRegistry clears the package-wide breaker list. Test helper.
func ResetBreakerRegistry() {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	breakers = nil
}
