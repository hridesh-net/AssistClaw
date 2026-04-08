package adapter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

func TestStub_implementsAdapter(t *testing.T) {
	t.Helper()
	var a Adapter = &Stub{ChannelName: "test-channel"}
	if a.Name() != "test-channel" {
		t.Fatalf("Name: got %q", a.Name())
	}
	if a.AdapterVersion() != Version1 {
		t.Fatalf("AdapterVersion: got %d", a.AdapterVersion())
	}
	ctx := context.Background()
	if err := a.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := a.StartInbound(ctx, func(_ context.Context, _ InboundEvent) error { return nil }); err != nil {
		t.Fatalf("StartInbound: %v", err)
	}
	t.Cleanup(func() { _ = a.Stop() })

	rec, err := a.Send(ctx, OutboundMessage{IdempotencyKey: "k1", SessionID: "s1", Text: "hi"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rec.IdempotencyKey != "k1" || rec.ProviderMessageID == "" {
		t.Fatalf("receipt: %+v", rec)
	}
}

func TestChannelError_kindAndWrapping(t *testing.T) {
	t.Helper()
	base := NewChannelError(ErrorKindRetryable, "outer", errors.New("inner"))
	if KindOf(base) != ErrorKindRetryable {
		t.Fatalf("KindOf direct: got %v", KindOf(base))
	}
	if !IsRetryable(base) {
		t.Fatal("IsRetryable")
	}
	wrapped := fmt.Errorf("context: %w", ErrRateLimited)
	if !IsRateLimited(wrapped) {
		t.Fatal("IsRateLimited wrapped")
	}
	if KindOf(wrapped) != ErrorKindRateLimited {
		t.Fatalf("KindOf wrapped: got %v", KindOf(wrapped))
	}
	if IsPermanent(wrapped) {
		t.Fatal("should not be permanent")
	}
}

func TestInboundEvent_fields(t *testing.T) {
	t.Helper()
	ev := InboundEvent{
		ID:        "m1",
		ChannelID: "telegram",
		SessionID: "tg:1",
		OccurredAt: time.Now().UTC(),
		Sender:    SenderRef{ID: "u1", Username: "a"},
		Recipient: RecipientRef{ID: "c1", Kind: "dm"},
		Text:      "hello",
		Parts:     []provider.ContentPart{{Type: provider.ContentTypeText, Text: "hello"}},
		ThreadRef: &ThreadRef{ID: "t1"},
		Attachments: []Attachment{{ID: "f1", Kind: "image", MimeType: "image/png"}},
		Metadata:  map[string]string{"raw_chat_id": "1"},
	}
	if ev.Parts[0].Text != "hello" {
		t.Fatal("parts")
	}
}
