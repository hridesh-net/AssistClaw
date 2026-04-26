package email

import (
	"context"
	"strings"
	"testing"

	"github.com/assistclaw/assistclaw/internal/config"
	"go.uber.org/zap"
)

func TestParseInboundCommand(t *testing.T) {
	v, tok, body := ParseInboundCommand("  approve abcdef01  ")
	if v != "approve" || tok != "abcdef01" || body != "" {
		t.Fatalf("approve: got verb=%q tok=%q body=%q", v, tok, body)
	}
	v, tok, body = ParseInboundCommand("reject fedcba98")
	if v != "reject" || tok != "fedcba98" || body != "" {
		t.Fatalf("reject: got verb=%q tok=%q body=%q", v, tok, body)
	}
	v, tok, body = ParseInboundCommand("edit aabbccdd: Thanks, will do Monday.")
	if v != "edit" || tok != "aabbccdd" || body != "Thanks, will do Monday." {
		t.Fatalf("edit: got verb=%q tok=%q body=%q", v, tok, body)
	}
	v, _, _ = ParseInboundCommand("not a command")
	if v != "" {
		t.Fatalf("expected empty verb, got %q", v)
	}
}

func TestHandleInboundCommandRefusesDeleteKeyword(t *testing.T) {
	td := t.TempDir()
	st, err := OpenStore(td)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := &config.Config{
		StateDir: td,
		Email: config.EmailConfig{
			Enabled: true,
			Notify: config.EmailNotifyConfig{
				Channel:   "telegram",
				SessionID: "tg:1",
			},
			Accounts: []config.EmailAccountConfig{
				{
					Name:    "acc",
					Backend: "imap",
					IMAP: &config.EmailIMAPConfig{
						Host:     "127.0.0.1:1993",
						Username: "u",
						Password: "p",
					},
					SMTP: &config.EmailSMTPConfig{
						Host:     "127.0.0.1",
						Port:     1025,
						Username: "u",
						Password: "p",
					},
				},
			},
		},
	}
	svc, err := NewService(cfg, st, nil, "test-model", nil, nil, zap.NewNop())
	if err != nil || svc == nil {
		t.Fatalf("NewService: err=%v svc=%v", err, svc)
	}
	reply, handled, err := svc.HandleInboundCommand(context.Background(), "telegram", "tg:1", "approve abcdef01\ndelete thread")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected handled")
	}
	if reply == "" || !strings.Contains(strings.ToLower(reply), "delete") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestInboundMatchesNotifyPerAccount(t *testing.T) {
	td := t.TempDir()
	st, err := OpenStore(td)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := &config.Config{
		StateDir: td,
		Email: config.EmailConfig{
			Enabled: true,
			Notify: config.EmailNotifyConfig{
				Channel:   "telegram",
				SessionID: "tg:1",
			},
			Accounts: []config.EmailAccountConfig{
				{
					Name:    "acc",
					Backend: "imap",
					Notify: &config.EmailNotifyConfig{
						Channel:   "slack",
						SessionID: "slack:C09:1.2",
					},
					IMAP: &config.EmailIMAPConfig{Host: "127.0.0.1:1993", Username: "u", Password: "p"},
					SMTP: &config.EmailSMTPConfig{Host: "127.0.0.1", Port: 1025, Username: "u", Password: "p"},
				},
			},
		},
	}
	svc, err := NewService(cfg, st, nil, "m", nil, nil, zap.NewNop())
	if err != nil || svc == nil {
		t.Fatal(err)
	}
	if !svc.inboundMatchesNotify("slack", "slack:C09:1.2") {
		t.Fatal("expected per-account notify session to match")
	}
	if svc.inboundMatchesNotify("slack", "slack:other") {
		t.Fatal("should not match wrong session")
	}
}
