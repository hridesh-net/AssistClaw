package email

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type gmailBackend struct {
	cfg   *config.Config
	acc   config.EmailAccountConfig
	store *Store
	srv   *gmail.Service
}

func init() {
	RegisterBackendBuilder("gmail", newGmailBackend)
}

func newGmailBackend(cfg *config.Config, acc config.EmailAccountConfig, store *Store) (Backend, error) {
	if acc.Gmail == nil || strings.TrimSpace(acc.Gmail.TokenFile) == "" {
		return nil, fmt.Errorf("gmail token_file required")
	}
	tok, err := loadTokenFromFile(cfg.StateDir, acc.Gmail.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("gmail oauth token: %w", err)
	}
	clientID := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_ID")
	clientSecret := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_SECRET")
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailSendScope},
	}
	ctx := context.Background()
	ts := oauthCfg.TokenSource(ctx, tok)
	srv, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &gmailBackend{cfg: cfg, acc: acc, store: store, srv: srv}, nil
}

func (b *gmailBackend) Name() string { return "gmail-" + b.acc.Name }

func (b *gmailBackend) Watch(ctx context.Context, onNew func(context.Context, Ref) error) error {
	iv, err := time.ParseDuration(b.cfg.Email.PollInterval)
	if err != nil {
		iv = 60 * time.Second
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		_ = b.poll(ctx, onNew)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (b *gmailBackend) poll(ctx context.Context, onNew func(context.Context, Ref) error) error {
	r, err := b.srv.Users.Messages.List("me").LabelIds("INBOX").MaxResults(15).Do()
	if err != nil {
		return err
	}
	for _, m := range r.Messages {
		if m.Id == "" {
			continue
		}
		exists, err := b.store.MessageExists(b.acc.Name, m.Id)
		if err != nil || exists {
			continue
		}
		if err := onNew(ctx, Ref{AccountName: b.acc.Name, ProviderID: m.Id}); err != nil {
			return err
		}
	}
	return nil
}

func (b *gmailBackend) Fetch(ctx context.Context, ref Ref) (*MailMessage, error) {
	msg, err := b.srv.Users.Messages.Get("me", ref.ProviderID).Format("full").Do()
	if err != nil {
		return nil, err
	}
	from, subj, body, mid, irt, refs := parseGmailMessage(msg)
	labels := msg.LabelIds
	return &MailMessage{
		Ref:         ref,
		From:        from,
		Subject:     subj,
		BodyText:    body,
		MessageID:   mid,
		InReplyTo:   irt,
		References:  refs,
		GmailLabels: labels,
	}, nil
}

func parseGmailMessage(msg *gmail.Message) (from, subj, body, msgID, inReplyTo, refs string) {
	for _, h := range msg.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			from = h.Value
		case "subject":
			subj = h.Value
		case "message-id":
			msgID = h.Value
		case "in-reply-to":
			inReplyTo = h.Value
		case "references":
			refs = h.Value
		}
	}
	body = extractGmailPlain(msg.Payload)
	return
}

func extractGmailPlain(p *gmail.MessagePart) string {
	if p == nil {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(p.MimeType), "text/plain") && p.Body != nil && p.Body.Data != "" {
		b, err := decodeGmailWeb64(p.Body.Data)
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	for _, c := range p.Parts {
		if s := extractGmailPlain(c); s != "" {
			return s
		}
	}
	return ""
}

func decodeGmailWeb64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.URLEncoding.DecodeString(s)
}

func (b *gmailBackend) Reply(ctx context.Context, m *MailMessage, body string) error {
	addr, err := mail.ParseAddress(m.From)
	if err != nil {
		addr = &mail.Address{Address: strings.Trim(m.From, "<>")}
	}
	subj := m.Subject
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	var raw strings.Builder
	fmt.Fprintf(&raw, "To: %s\r\n", addr.String())
	fmt.Fprintf(&raw, "Subject: %s\r\n", subj)
	if m.MessageID != "" {
		fmt.Fprintf(&raw, "In-Reply-To: %s\r\n", strings.TrimSpace(m.MessageID))
		r := m.References
		if r == "" {
			r = m.MessageID
		} else {
			r = strings.TrimSpace(r) + " " + strings.TrimSpace(m.MessageID)
		}
		fmt.Fprintf(&raw, "References: %s\r\n", r)
	}
	fmt.Fprintf(&raw, "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", body)
	enc := base64.URLEncoding.EncodeToString([]byte(raw.String()))
	gmsg := &gmail.Message{Raw: enc}
	_, err = b.srv.Users.Messages.Send("me", gmsg).Do()
	return err
}
