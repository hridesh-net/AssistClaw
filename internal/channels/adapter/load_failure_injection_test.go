package adapter

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/observability/metrics"
)

type loadSender struct {
	failFirstN int64
	delay      time.Duration
	calls      int64
}

func (s *loadSender) Send(_ context.Context, msg OutboundMessage) (*DeliveryReceipt, error) {
	call := atomic.AddInt64(&s.calls, 1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if call <= s.failFirstN {
		// Simulate 429/500 style transient failure.
		return nil, NewChannelError(ErrorKindRetryable, "simulated 429/500", nil)
	}
	return &DeliveryReceipt{
		ProviderMessageID: fmt.Sprintf("msg-%d", call),
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

type loadReport struct {
	total     int
	succeeded int
	failed    int
	latencies []time.Duration
}

func runLoadScenario(t *testing.T, sender *ReliableSender, total int, workers int) loadReport {
	t.Helper()
	if workers <= 0 {
		workers = 1
	}
	var rep loadReport
	rep.total = total
	rep.latencies = make([]time.Duration, 0, total)

	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				start := time.Now()
				_, err := sender.Send(context.Background(), OutboundMessage{
					SessionID: fmt.Sprintf("load:%d", i),
					Text:      "hello",
				})
				d := time.Since(start)
				mu.Lock()
				rep.latencies = append(rep.latencies, d)
				if err != nil {
					rep.failed++
				} else {
					rep.succeeded++
				}
				mu.Unlock()
			}
		}()
	}

	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return rep
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	cp := make([]time.Duration, len(durations))
	copy(cp, durations)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(float64(len(cp)-1) * p)
	return cp[idx]
}

func TestLoadHarness_Smoke(t *testing.T) {
	t.Helper()
	metrics.Default().ResetForTests()
	beforeG := runtime.NumGoroutine()

	inner := &loadSender{failFirstN: 2, delay: 2 * time.Millisecond}
	rs := NewReliableSender("telegram", inner, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   3,
			BaseDelay:     time.Millisecond,
			MaxDelay:      2 * time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 10,
			Cooldown:         50 * time.Millisecond,
		},
	})
	rs.sleepFn = func(context.Context, time.Duration) error { return nil }

	rep := runLoadScenario(t, rs, 30, 3)
	if rep.succeeded == 0 {
		t.Fatal("smoke scenario expected successful sends")
	}
	if rep.failed > 5 {
		t.Fatalf("smoke scenario too many failures: %d", rep.failed)
	}

	rendered := metrics.Default().RenderPrometheus()
	if !strings.Contains(rendered, `messages_sent_total{channel="telegram"}`) {
		t.Fatalf("expected sent metric update, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `adapter_retries_total{channel="telegram"}`) {
		t.Fatalf("expected retries metric update, got:\n%s", rendered)
	}

	// Best-effort leak check: allow a small delta for runtime/background goroutines.
	afterG := runtime.NumGoroutine()
	if afterG > beforeG+12 {
		t.Fatalf("possible goroutine leak: before=%d after=%d", beforeG, afterG)
	}
}

func TestLoadHarness_Report(t *testing.T) {
	t.Helper()
	metrics.Default().ResetForTests()

	steadySender := NewReliableSender("telegram", &loadSender{delay: 3 * time.Millisecond}, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   2,
			BaseDelay:     time.Millisecond,
			MaxDelay:      2 * time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 50,
			Cooldown:         100 * time.Millisecond,
		},
	})
	steadySender.sleepFn = func(context.Context, time.Duration) error { return nil }
	steady := runLoadScenario(t, steadySender, 120, 6)

	burstSender := NewReliableSender("telegram", &loadSender{delay: 1 * time.Millisecond}, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   2,
			BaseDelay:     time.Millisecond,
			MaxDelay:      2 * time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 100,
			Cooldown:         100 * time.Millisecond,
		},
	})
	burstSender.sleepFn = func(context.Context, time.Duration) error { return nil }
	burst := runLoadScenario(t, burstSender, 240, 24)

	// Sustained failure -> breaker should open.
	failInner := &loadSender{failFirstN: 10}
	breakerSender := NewReliableSender("telegram", failInner, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   1,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 2,
			Cooldown:         20 * time.Millisecond,
		},
	})
	now := time.Now().UTC()
	breakerSender.nowFn = func() time.Time { return now }
	breakerSender.sleepFn = func(context.Context, time.Duration) error { return nil }
	_, _ = breakerSender.Send(context.Background(), OutboundMessage{SessionID: "f1", Text: "x"})
	_, _ = breakerSender.Send(context.Background(), OutboundMessage{SessionID: "f2", Text: "x"})
	callsAfterOpen := atomic.LoadInt64(&failInner.calls)
	_, err := breakerSender.Send(context.Background(), OutboundMessage{SessionID: "f3", Text: "x"})
	if err == nil {
		t.Fatal("expected breaker-open failure")
	}
	if atomic.LoadInt64(&failInner.calls) != callsAfterOpen {
		t.Fatalf("breaker-open should block downstream call; calls=%d->%d", callsAfterOpen, atomic.LoadInt64(&failInner.calls))
	}
	// Recovery after cooldown.
	now = now.Add(25 * time.Millisecond)
	_, _ = breakerSender.Send(context.Background(), OutboundMessage{SessionID: "recover", Text: "x"})

	t.Logf("steady: total=%d success=%d failed=%d p50=%s p95=%s p99=%s",
		steady.total, steady.succeeded, steady.failed,
		percentile(steady.latencies, 0.50), percentile(steady.latencies, 0.95), percentile(steady.latencies, 0.99))
	t.Logf("burst: total=%d success=%d failed=%d p50=%s p95=%s p99=%s",
		burst.total, burst.succeeded, burst.failed,
		percentile(burst.latencies, 0.50), percentile(burst.latencies, 0.95), percentile(burst.latencies, 0.99))
}
