package telegram

import "testing"

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
