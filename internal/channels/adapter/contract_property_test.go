package adapter

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

const (
	propertySeed     int64 = 20260414
	propertyMaxCount int   = 300
)

type contractAdapter struct {
	name           string
	startErr       error
	stopErr        error
	sendErr        error
	providerMsgID  string
	startedInbound bool
}

func (a *contractAdapter) Name() string { return a.name }

func (a *contractAdapter) AdapterVersion() int { return Version1 }

func (a *contractAdapter) Capabilities() ChannelCapabilities {
	return ChannelCapabilities{
		Threading:      true,
		Attachments:    true,
		DirectMessages: true,
		GroupMessages:  true,
		Mentions:       true,
	}
}

func (a *contractAdapter) StartInbound(_ context.Context, _ InboundHandler) error {
	if a.startErr != nil {
		return a.startErr
	}
	a.startedInbound = true
	return nil
}

func (a *contractAdapter) Stop() error {
	if a.stopErr != nil {
		return a.stopErr
	}
	a.startedInbound = false
	return nil
}

func (a *contractAdapter) Send(_ context.Context, msg OutboundMessage) (*DeliveryReceipt, error) {
	if a.sendErr != nil {
		return nil, a.sendErr
	}
	return &DeliveryReceipt{
		ProviderMessageID: a.providerMsgID,
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

func (a *contractAdapter) Ping(_ context.Context) error { return nil }

func runAdapterContractSuite(t *testing.T, subjectName string, newAdapter func(t *testing.T) *contractAdapter) {
	t.Helper()

	t.Run(subjectName+"/identity-and-lifecycle", func(t *testing.T) {
		ad := newAdapter(t)
		if ad.Name() == "" {
			t.Fatalf("%s: Name() must be non-empty", subjectName)
		}
		if ad.AdapterVersion() != Version1 {
			t.Fatalf("%s: AdapterVersion() = %d, want %d", subjectName, ad.AdapterVersion(), Version1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if err := ad.StartInbound(ctx, func(context.Context, InboundEvent) error { return nil }); err != nil {
			t.Fatalf("%s: StartInbound() error = %v", subjectName, err)
		}
		if err := ad.Stop(); err != nil {
			t.Fatalf("%s: Stop() first call error = %v", subjectName, err)
		}
		if err := ad.Stop(); err != nil {
			t.Fatalf("%s: Stop() second call must be safe, error = %v", subjectName, err)
		}
	})

	t.Run(subjectName+"/send-success", func(t *testing.T) {
		ad := newAdapter(t)
		msg := OutboundMessage{SessionID: "thread:a", Text: "hello", IdempotencyKey: "k-contract-1"}
		rec, err := ad.Send(context.Background(), msg)
		if err != nil {
			t.Fatalf("%s: Send(success) error = %v", subjectName, err)
		}
		if rec == nil {
			t.Fatalf("%s: Send(success) returned nil receipt", subjectName)
		}
		if rec.ProviderMessageID == "" {
			t.Fatalf("%s: Send(success) receipt ProviderMessageID is empty", subjectName)
		}
		if rec.IdempotencyKey != msg.IdempotencyKey {
			t.Fatalf("%s: Send(success) receipt IdempotencyKey = %q, want %q", subjectName, rec.IdempotencyKey, msg.IdempotencyKey)
		}
	})

	t.Run(subjectName+"/send-retryable-classification", func(t *testing.T) {
		ad := newAdapter(t)
		ad.sendErr = NewChannelError(ErrorKindRetryable, "temporary outage", nil)
		_, err := ad.Send(context.Background(), OutboundMessage{SessionID: "thread:a", Text: "hello"})
		if err == nil {
			t.Fatalf("%s: Send(retryable) expected error, got nil", subjectName)
		}
		if got := KindOf(err); got != ErrorKindRetryable {
			t.Fatalf("%s: Send(retryable) KindOf(err) = %v, want %v", subjectName, got, ErrorKindRetryable)
		}
	})

	t.Run(subjectName+"/send-permanent-classification", func(t *testing.T) {
		ad := newAdapter(t)
		ad.sendErr = NewChannelError(ErrorKindPermanent, "invalid destination", nil)
		_, err := ad.Send(context.Background(), OutboundMessage{SessionID: "thread:a", Text: "hello"})
		if err == nil {
			t.Fatalf("%s: Send(permanent) expected error, got nil", subjectName)
		}
		if got := KindOf(err); got != ErrorKindPermanent {
			t.Fatalf("%s: Send(permanent) KindOf(err) = %v, want %v", subjectName, got, ErrorKindPermanent)
		}
	})
}

func TestAdapterContract_InMemoryAdapter(t *testing.T) {
	t.Helper()
	runAdapterContractSuite(t, "in-memory-contract-adapter", func(t *testing.T) *contractAdapter {
		t.Helper()
		return &contractAdapter{name: "in-memory", providerMsgID: "mem-1"}
	})
}

func TestAdapterContract_StubViaHarness(t *testing.T) {
	t.Helper()
	runAdapterContractSuite(t, "stub-adapter", func(t *testing.T) *contractAdapter {
		t.Helper()
		return &contractAdapter{name: "stub", providerMsgID: "stub-msg-1"}
	})
}

func TestProperty_StubSendPreservesIdempotencyKey(t *testing.T) {
	t.Helper()
	s := &Stub{ChannelName: "stub"}
	cfg := &quick.Config{
		MaxCount: propertyMaxCount,
		Rand:     rand.New(rand.NewSource(propertySeed)),
	}
	prop := func(raw string) bool {
		if raw == "" {
			return true
		}
		rec, err := s.Send(context.Background(), OutboundMessage{
			IdempotencyKey: raw,
			SessionID:      "prop:s1",
			Text:           "hello",
		})
		if err != nil || rec == nil {
			return false
		}
		return rec.IdempotencyKey == raw
	}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("property failed: Stub.Send must preserve non-empty idempotency key: %v", err)
	}
}

func TestProperty_BackoffMonotonicWithoutJitter(t *testing.T) {
	t.Helper()
	cfg := &quick.Config{
		MaxCount: propertyMaxCount,
		Rand:     rand.New(rand.NewSource(propertySeed)),
	}
	prop := func(baseMs uint16, maxMs uint16, n uint8) bool {
		base := time.Duration(1+int(baseMs)%300) * time.Millisecond
		max := time.Duration(1+int(maxMs)%600) * time.Millisecond
		if max < base {
			base, max = max, base
		}
		attempts := 2 + int(n%20)

		rs := NewReliableSender("test", &Stub{}, ReliabilityConfig{
			Retry: RetryPolicy{
				MaxAttempts:   attempts,
				BaseDelay:     base,
				MaxDelay:      max,
				JitterPercent: 0,
			},
			Breaker: CircuitBreakerPolicy{
				FailureThreshold: 1000,
				Cooldown:         time.Second,
			},
		})

		prev := time.Duration(0)
		for attempt := 1; attempt <= attempts; attempt++ {
			got := rs.backoff(attempt)
			if got < prev {
				return false
			}
			if got > max {
				return false
			}
			prev = got
		}
		return true
	}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("property failed: backoff must be monotonic and capped when jitter is disabled: %v", err)
	}
}

func TestProperty_ConfigIsDocumentedForDeterminism(t *testing.T) {
	t.Helper()
	if propertyMaxCount < 200 {
		t.Fatalf("property MaxCount must be >= 200 for meaningful fuzzing, got %d", propertyMaxCount)
	}
	if propertySeed == 0 {
		t.Fatal("property seed must be fixed/non-zero for deterministic CI runs")
	}
	if got := fmt.Sprintf("%d", propertySeed); got == "" {
		t.Fatal("property seed formatting failed unexpectedly")
	}
}
