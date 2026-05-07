package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
)

// ─────────────────────────────────────────────
// Session ID helpers
// ─────────────────────────────────────────────

func TestTeamsSessionIDRoundtrip(t *testing.T) {
	serviceURL := "https://smba.trafficmanager.net/amer"
	convID := "a:1abc123"

	sid := teamsSessionID(serviceURL, convID)
	gotSvc, gotConv, err := parseTeamsSession(sid)
	if err != nil {
		t.Fatalf("parseTeamsSession: %v", err)
	}
	if gotSvc != serviceURL {
		t.Errorf("serviceURL: got %q, want %q", gotSvc, serviceURL)
	}
	if gotConv != convID {
		t.Errorf("convID: got %q, want %q", gotConv, convID)
	}
}

func TestParseTeamsSession_InvalidPrefix(t *testing.T) {
	_, _, err := parseTeamsSession("telegram:foo:bar")
	if err == nil {
		t.Fatal("expected error for wrong prefix")
	}
}

func TestParseTeamsSession_MissingColon(t *testing.T) {
	_, _, err := parseTeamsSession("teams:nocolon")
	if err == nil {
		t.Fatal("expected error for missing @ separator")
	}
}

// ─────────────────────────────────────────────
// splitMessage
// ─────────────────────────────────────────────

func TestSplitMessage_ShortMessage(t *testing.T) {
	parts := splitMessage("hello world", 100)
	if len(parts) != 1 || parts[0] != "hello world" {
		t.Errorf("unexpected parts: %v", parts)
	}
}

func TestSplitMessage_Empty(t *testing.T) {
	if len(splitMessage("", 100)) != 0 {
		t.Error("expected empty result for empty input")
	}
}

func TestSplitMessage_LongMessage(t *testing.T) {
	// Build a string longer than maxLen with spaces so it can be split.
	word := strings.Repeat("a", 50) + " "
	msg := strings.Repeat(word, 30) // 51*30 = 1530 chars
	parts := splitMessage(msg, 200)
	if len(parts) < 2 {
		t.Errorf("expected multiple parts, got %d", len(parts))
	}
	for _, p := range parts {
		if len(p) > 200 {
			t.Errorf("part exceeds maxLen: len=%d", len(p))
		}
	}
}

// ─────────────────────────────────────────────
// outboundBody
// ─────────────────────────────────────────────

func TestOutboundBody_TextDirect(t *testing.T) {
	msg := adapter.OutboundMessage{Text: "hello"}
	if got := outboundBody(msg); got != "hello" {
		t.Errorf("got %q", got)
	}
}

// ─────────────────────────────────────────────
// shouldAccept
// ─────────────────────────────────────────────

func TestShouldAccept_Open(t *testing.T) {
	c := &Channel{dmMode: "open"}
	act := activity{From: account{ID: "user1"}}
	if !c.shouldAccept(act) {
		t.Error("open mode should accept all")
	}
}

func TestShouldAccept_Disabled(t *testing.T) {
	c := &Channel{dmMode: "disabled"}
	act := activity{From: account{ID: "user1"}}
	if c.shouldAccept(act) {
		t.Error("disabled mode should reject all")
	}
}

func TestShouldAccept_Allowlist_Allowed(t *testing.T) {
	c := &Channel{dmMode: "allowlist", allowFrom: []string{"user1", "user2"}}
	act := activity{From: account{ID: "user1"}}
	if !c.shouldAccept(act) {
		t.Error("allowlisted user should be accepted")
	}
}

func TestShouldAccept_Allowlist_Blocked(t *testing.T) {
	c := &Channel{dmMode: "allowlist", allowFrom: []string{"user1"}}
	act := activity{From: account{ID: "stranger"}}
	if c.shouldAccept(act) {
		t.Error("non-allowlisted user should be blocked")
	}
}

// ─────────────────────────────────────────────
// HTTP inbound handler (StartInbound)
// ─────────────────────────────────────────────

func TestStartInbound_ReceivesMessage(t *testing.T) {
	ch, err := New("appid", "appsecret", ":0", "open", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	received := make(chan adapter.InboundEvent, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ch.StartInbound(ctx, func(_ context.Context, ev adapter.InboundEvent) error {
		received <- ev
		return nil
	}); err != nil {
		t.Fatalf("StartInbound: %v", err)
	}
	defer ch.Stop()

	// Build a fake activity payload.
	act := activity{
		Type:       "message",
		ID:         "act1",
		ServiceURL: "https://smba.trafficmanager.net/amer",
		From:       account{ID: "user1", Name: "Alice"},
		Conversation: conversationAccount{
			ID:               "conv1",
			ConversationType: "personal",
		},
		Text: "hello teams",
	}
	body, _ := json.Marshal(act)

	// Post directly to the handler via httptest (bypasses the real listener).
	req := httptest.NewRequest(http.MethodPost, "/teams/messages", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ch.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	select {
	case ev := <-received:
		if ev.Text != "hello teams" {
			t.Errorf("text: got %q", ev.Text)
		}
		if ev.SessionID == "" {
			t.Error("session id should not be empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound event")
	}
}

func TestStartInbound_IgnoresNonMessage(t *testing.T) {
	ch, err := New("appid", "appsecret", ":0", "open", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	called := false
	_ = ch.StartInbound(ctx, func(_ context.Context, _ adapter.InboundEvent) error {
		called = true
		return nil
	})
	defer ch.Stop()

	act := activity{Type: "typing", ID: "t1", ServiceURL: "https://example.com"}
	body, _ := json.Marshal(act)
	req := httptest.NewRequest(http.MethodPost, "/teams/messages", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	ch.server.Handler.ServeHTTP(rr, req)

	time.Sleep(50 * time.Millisecond)
	if called {
		t.Error("handler should not be called for non-message activity")
	}
}
