package slack

import (
	"context"
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
)

type captureSlackSender struct {
	msgs []adapter.OutboundMessage
}

func (c *captureSlackSender) Send(_ context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	c.msgs = append(c.msgs, msg)
	return &adapter.DeliveryReceipt{
		ProviderMessageID: "ok",
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

func TestParseSlackSession(t *testing.T) {
	t.Helper()
	ch, thread, err := parseSlackSession("slack:C123:1744661304.000100")
	if err != nil {
		t.Fatalf("parseSlackSession: %v", err)
	}
	if ch != "C123" || thread != "1744661304.000100" {
		t.Fatalf("unexpected parsed values: %q, %q", ch, thread)
	}

	_, _, err = parseSlackSession("bad:C123")
	if err == nil {
		t.Fatal("expected invalid prefix error")
	}
}

func TestSendLegacyReplyChunk_UsesReliableSender(t *testing.T) {
	t.Helper()
	inner := &captureSlackSender{}
	rs := adapter.NewReliableSender("slack", inner, adapter.ReliabilityConfig{
		Retry: adapter.RetryPolicy{
			MaxAttempts:   2,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			JitterPercent: 0,
		},
		Breaker: adapter.CircuitBreakerPolicy{
			FailureThreshold: 10,
			Cooldown:         time.Second,
		},
	})
	ch := (&Channel{}).WithReliableOutbound(rs)

	err := ch.sendLegacyReplyChunk(context.Background(), "C123", "1744661304.000100", "hello")
	if err != nil {
		t.Fatalf("sendLegacyReplyChunk: %v", err)
	}
	if len(inner.msgs) != 1 {
		t.Fatalf("expected one outbound message, got %d", len(inner.msgs))
	}
	if inner.msgs[0].SessionID != "slack:C123:1744661304.000100" {
		t.Fatalf("unexpected session id: %q", inner.msgs[0].SessionID)
	}
}
