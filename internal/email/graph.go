package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
	"golang.org/x/oauth2"
)

var msEndpoint = oauth2.Endpoint{
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
}

type graphBackend struct {
	cfg    *config.Config
	acc    config.EmailAccountConfig
	store  *Store
	client *http.Client
}

func init() {
	RegisterBackendBuilder("graph", newGraphBackend)
}

func newGraphBackend(cfg *config.Config, acc config.EmailAccountConfig, store *Store) (Backend, error) {
	if acc.Graph == nil || strings.TrimSpace(acc.Graph.TokenFile) == "" {
		return nil, fmt.Errorf("graph token_file required")
	}
	tok, err := loadTokenFromFile(cfg.StateDir, acc.Graph.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("graph oauth token: %w", err)
	}
	clientID := os.Getenv("ASSISTCLAW_GRAPH_CLIENT_ID")
	clientSecret := os.Getenv("ASSISTCLAW_GRAPH_CLIENT_SECRET")
	if clientID == "" {
		clientID = os.Getenv("MICROSOFT_OAUTH_CLIENT_ID")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("MICROSOFT_OAUTH_CLIENT_SECRET")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     msEndpoint,
		Scopes: []string{
			"https://graph.microsoft.com/Mail.Read",
			"https://graph.microsoft.com/Mail.Send",
			"offline_access",
		},
	}
	ctx := context.Background()
	ts := oauthCfg.TokenSource(ctx, tok)
	return &graphBackend{
		cfg:    cfg,
		acc:    acc,
		store:  store,
		client: oauth2.NewClient(ctx, ts),
	}, nil
}

func (b *graphBackend) Name() string { return "graph-" + b.acc.Name }

func (b *graphBackend) Watch(ctx context.Context, onNew func(context.Context, Ref) error) error {
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

func (b *graphBackend) poll(ctx context.Context, onNew func(context.Context, Ref) error) error {
	u := "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?$top=15&$orderby=receivedDateTime%20desc"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graph list: %s: %s", resp.Status, string(bb))
	}
	var out struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	for _, m := range out.Value {
		if m.ID == "" {
			continue
		}
		exists, err := b.store.MessageExists(b.acc.Name, m.ID)
		if err != nil || exists {
			continue
		}
		if err := onNew(ctx, Ref{AccountName: b.acc.Name, ProviderID: m.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (b *graphBackend) Fetch(ctx context.Context, ref Ref) (*MailMessage, error) {
	u := "https://graph.microsoft.com/v1.0/me/messages/" + url.PathEscape(ref.ProviderID) +
		"?$select=subject,body,from,internetMessageHeaders,singleValueExtendedProperties"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph get: %s: %s", resp.Status, string(bb))
	}
	var gm struct {
		Subject string `json:"subject"`
		Body    struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		From struct {
			EmailAddress struct {
				Name    string `json:"name"`
				Address string `json:"address"`
			} `json:"emailAddress"`
		} `json:"from"`
		InternetMessageHeaders []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"internetMessageHeaders"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gm); err != nil {
		return nil, err
	}
	from := gm.From.EmailAddress.Address
	if gm.From.EmailAddress.Name != "" {
		from = fmt.Sprintf("%s <%s>", gm.From.EmailAddress.Name, gm.From.EmailAddress.Address)
	}
	body := strings.TrimSpace(gm.Body.Content)
	if strings.EqualFold(gm.Body.ContentType, "html") {
		body = stripHTMLTags(body)
	}
	headers := map[string]string{}
	for _, h := range gm.InternetMessageHeaders {
		headers[strings.ToLower(h.Name)] = h.Value
	}
	return &MailMessage{
		Ref:        ref,
		From:       from,
		Subject:    gm.Subject,
		BodyText:   body,
		MessageID:  headers["message-id"],
		InReplyTo:  headers["in-reply-to"],
		References: headers["references"],
	}, nil
}

func stripHTMLTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '<' {
			in = true
			continue
		}
		if r == '>' {
			in = false
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func (b *graphBackend) Reply(ctx context.Context, m *MailMessage, body string) error {
	addr, err := mail.ParseAddress(m.From)
	if err != nil {
		addr = &mail.Address{Address: strings.Trim(m.From, "<>")}
	}
	subj := m.Subject
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	msg := map[string]any{
		"subject": subj,
		"body": map[string]any{
			"contentType": "text",
			"content":     body,
		},
		"toRecipients": []any{
			map[string]any{
				"emailAddress": map[string]any{
					"address": addr.Address,
					"name":    addr.Name,
				},
			},
		},
	}
	if m.MessageID != "" {
		msg["internetMessageHeaders"] = []any{
			map[string]string{"name": "In-Reply-To", "value": strings.TrimSpace(m.MessageID)},
		}
	}
	payload := map[string]any{"message": msg}
	raw, _ := json.Marshal(payload)
	u := "https://graph.microsoft.com/v1.0/me/sendMail"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graph send: %s: %s", resp.Status, string(bb))
	}
	return nil
}
