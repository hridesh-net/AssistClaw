package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type flakySender struct {
	failCount int
	calls     int
	errKind   ErrorKind
}

func (f *flakySender) Send(_ context.Context, msg OutboundMessage) (*DeliveryReceipt, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, NewChannelError(f.errKind, "simulated failure", nil)
	}
	return &DeliveryReceipt{
		ProviderMessageID: "ok",
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

func TestReliableSender_RetryThenSuccess(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	inner := &flakySender{failCount: 2, errKind: ErrorKindRetryable}
	rs := NewReliableSender("telegram", inner, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   4,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 10,
			Cooldown:         time.Second,
		},
		DLQPath: filepath.Join(tmp, "dlq.ndjson"),
	})
	rs.sleepFn = func(context.Context, time.Duration) error { return nil }
	rs.randFn = func() float64 { return 0.5 }

	rec, err := rs.Send(context.Background(), OutboundMessage{
		SessionID: "tg:1",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if rec == nil || rec.ProviderMessageID == "" {
		t.Fatalf("expected receipt, got %+v", rec)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
	// Success path should not write DLQ.
	if b, _ := os.ReadFile(filepath.Join(tmp, "dlq.ndjson")); len(strings.TrimSpace(string(b))) != 0 {
		t.Fatalf("unexpected DLQ content: %s", string(b))
	}
}

func TestReliableSender_PermanentWritesDLQ(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	inner := &flakySender{failCount: 10, errKind: ErrorKindPermanent}
	rs := NewReliableSender("slack", inner, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   5,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 3,
			Cooldown:         time.Second,
		},
		DLQPath: filepath.Join(tmp, "dlq.ndjson"),
	})
	rs.sleepFn = func(context.Context, time.Duration) error { return nil }

	_, err := rs.Send(context.Background(), OutboundMessage{
		SessionID:      "slack:C1",
		Text:           "hello",
		IdempotencyKey: "k1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("permanent error should not retry, calls=%d", inner.calls)
	}
	b, readErr := os.ReadFile(filepath.Join(tmp, "dlq.ndjson"))
	if readErr != nil {
		t.Fatalf("expected DLQ file, err=%v", readErr)
	}
	s := string(b)
	if !strings.Contains(s, `"channel":"slack"`) || !strings.Contains(s, `"idempotency_key":"k1"`) {
		t.Fatalf("unexpected DLQ content: %s", s)
	}
}

func TestReliableSender_CircuitBreakerTransitions(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	inner := &flakySender{failCount: 10, errKind: ErrorKindRetryable}
	now := time.Now().UTC()
	rs := NewReliableSender("discord", inner, ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   1,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 2,
			Cooldown:         10 * time.Second,
		},
		DLQPath: filepath.Join(tmp, "dlq.ndjson"),
	})
	rs.nowFn = func() time.Time { return now }
	rs.sleepFn = func(context.Context, time.Duration) error { return nil }

	// two failures open breaker
	_, _ = rs.Send(context.Background(), OutboundMessage{SessionID: "discord::C", Text: "a"})
	_, _ = rs.Send(context.Background(), OutboundMessage{SessionID: "discord::C", Text: "b"})

	callsAfterOpen := inner.calls
	_, err := rs.Send(context.Background(), OutboundMessage{SessionID: "discord::C", Text: "c"})
	if err == nil {
		t.Fatal("expected breaker-open error")
	}
	if inner.calls != callsAfterOpen {
		t.Fatalf("breaker-open should block send call; got %d->%d", callsAfterOpen, inner.calls)
	}

	// move time after cooldown: half-open probe allowed (one call)
	now = now.Add(11 * time.Second)
	_, _ = rs.Send(context.Background(), OutboundMessage{SessionID: "discord::C", Text: "d"})
	if inner.calls != callsAfterOpen+1 {
		t.Fatalf("expected one half-open probe call, got %d", inner.calls)
	}
}
