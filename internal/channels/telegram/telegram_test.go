package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
)

func TestParseTelegramSession(t *testing.T) {
	t.Helper()
	target, err := parseTelegramSession("tg:12345")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if target.ChatID != 12345 || target.ThreadID != 0 {
		t.Fatalf("unexpected target: %+v", target)
	}
	if _, err := parseTelegramSession("bad:123"); err == nil {
		t.Fatal("expected invalid prefix error")
	}
	if _, err := parseTelegramSession("tg:abc"); err == nil {
		t.Fatal("expected invalid chat id error")
	}
}

func TestSplitTelegramMessage(t *testing.T) {
	t.Helper()
	in := "hello"
	parts := splitTelegramMessage(in)
	if len(parts) != 1 || parts[0] != in {
		t.Fatalf("unexpected split: %#v", parts)
	}

	// 5000 chars should split into at least 2 chunks.
	long := ""
	for i := 0; i < 5000; i++ {
		long += "a"
	}
	parts = splitTelegramMessage(long)
	if len(parts) < 2 {
		t.Fatalf("expected split for long payload, got %d", len(parts))
	}
	for _, p := range parts {
		if len(p) > 4096 {
			t.Fatalf("chunk exceeds telegram limit: %d", len(p))
		}
	}
}

type captureSender struct {
	msgs []adapter.OutboundMessage
}

func (c *captureSender) Send(_ context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	c.msgs = append(c.msgs, msg)
	return &adapter.DeliveryReceipt{
		ProviderMessageID: "ok",
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

func TestSendLegacyReplyText_UsesReliableSender(t *testing.T) {
	t.Helper()
	inner := &captureSender{}
	rs := adapter.NewReliableSender("telegram", inner, adapter.ReliabilityConfig{
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

	ch := &Channel{reliableSend: rs}
	err := ch.sendLegacyReplyText(context.Background(), 123, 456, "hello")
	if err != nil {
		t.Fatalf("sendLegacyReplyText: %v", err)
	}
	if len(inner.msgs) != 1 {
		t.Fatalf("expected one outbound message, got %d", len(inner.msgs))
	}
	if inner.msgs[0].SessionID != "tg:123" {
		t.Fatalf("unexpected session id: %q", inner.msgs[0].SessionID)
	}
	if inner.msgs[0].ReplyToID != "456" {
		t.Fatalf("unexpected reply id: %q", inner.msgs[0].ReplyToID)
	}
}
