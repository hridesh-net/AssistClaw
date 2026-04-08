package channels

import (
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/provider"
)

func TestMessageFromInbound(t *testing.T) {
	t.Helper()
	ev := adapter.InboundEvent{
		ID:         "1",
		ChannelID:  "telegram",
		SessionID:  "tg:99",
		OccurredAt: time.Unix(100, 0).UTC(),
		Text:       "hi",
		Parts:      []provider.ContentPart{{Type: provider.ContentTypeText, Text: "hi"}},
		Metadata:   map[string]string{"k": "v"},
	}
	m := MessageFromInbound(ev)
	if m.ID != "1" || m.ChannelID != "telegram" || m.SessionID != "tg:99" || m.Text != "hi" {
		t.Fatalf("unexpected message: %+v", m)
	}
	if m.Metadata["k"] != "v" {
		t.Fatalf("metadata: %v", m.Metadata)
	}
}
