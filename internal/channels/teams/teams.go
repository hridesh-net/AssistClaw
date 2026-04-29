// Package teams implements a Microsoft Teams channel for AssistClaw using the
// Bot Framework Activity schema over HTTP. AssistClaw acts as a Bot Framework
// bot: Teams POSTs Activity JSON to a configurable listen address, and replies
// are sent back via the serviceUrl in each activity.
//
// Setup (no SDK required):
//  1. Register an Azure Bot (App ID + Password) in the Azure portal.
//  2. Set the messaging endpoint to https://<your-host>/teams/messages.
//  3. Add the bot to a Teams channel or chat.
//  4. Set teams.app_id, teams.app_password, and teams.listen_addr in assistclaw.yaml.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// Compile-time checks.
var (
	_ adapter.Adapter  = (*Channel)(nil)
	_ channels.Channel = (*Channel)(nil)
)

// ─────────────────────────────────────────────
// Bot Framework Activity types (minimal subset)
// ─────────────────────────────────────────────

type account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type conversationAccount struct {
	ID               string `json:"id"`
	IsGroup          bool   `json:"isGroup"`
	ConversationType string `json:"conversationType"`
	Name             string `json:"name"`
}

type activity struct {
	Type         string              `json:"type"`
	ID           string              `json:"id"`
	Timestamp    string              `json:"timestamp,omitempty"`
	ServiceURL   string              `json:"serviceUrl"`
	ChannelID    string              `json:"channelId"`
	From         account             `json:"from"`
	Conversation conversationAccount `json:"conversation"`
	Recipient    account             `json:"recipient"`
	Text         string              `json:"text"`
	ReplyToID    string              `json:"replyToId,omitempty"`
}

// ─────────────────────────────────────────────
// Channel
// ─────────────────────────────────────────────

// Channel implements [channels.Channel] and [adapter.Adapter] for Microsoft Teams.
type Channel struct {
	appID        string
	appPassword  string
	listenAddr   string // e.g. ":3979" or "0.0.0.0:3979"
	dmMode       string
	allowFrom    []string
	reliableSend *adapter.ReliableSender

	server   *http.Server
	stopCh   chan struct{}
	stopOnce sync.Once

	// tokenMu guards the cached OAuth token.
	tokenMu    sync.Mutex
	tokenValue string
	tokenExp   time.Time
}

// New creates a Teams channel. listenAddr is the address to bind the HTTP listener on
// (e.g. ":3979"). appID and appPassword come from the Azure Bot registration.
func New(appID, appPassword, listenAddr, dmMode string, allowFrom []string) (*Channel, error) {
	if appID == "" || appPassword == "" {
		return nil, fmt.Errorf("teams: app_id and app_password are required")
	}
	if listenAddr == "" {
		listenAddr = ":3979"
	}
	return &Channel{
		appID:       appID,
		appPassword: appPassword,
		listenAddr:  listenAddr,
		dmMode:      dmMode,
		allowFrom:   allowFrom,
		stopCh:      make(chan struct{}),
	}, nil
}

// WithReliableOutbound wires the shared reliability wrapper for outbound sends.
func (c *Channel) WithReliableOutbound(rs *adapter.ReliableSender) *Channel {
	c.reliableSend = rs
	return c
}

// ─────────────────────────────────────────────
// adapter.Identity
// ─────────────────────────────────────────────

func (c *Channel) Name() string           { return "msteams" }
func (c *Channel) AdapterVersion() int    { return adapter.Version1 }
func (c *Channel) Capabilities() adapter.ChannelCapabilities {
	caps, ok := adapter.BuiltinCaps(c.Name())
	if !ok {
		panic("internal/channels/adapter: missing capability_registry entry for " + c.Name())
	}
	return caps
}

// ─────────────────────────────────────────────
// adapter.Health
// ─────────────────────────────────────────────

// Ping validates credentials by fetching an OAuth token.
func (c *Channel) Ping(ctx context.Context) error {
	_, err := c.getToken(ctx)
	if err != nil {
		return adapter.NewChannelError(adapter.ErrorKindPermanent, "teams ping: token fetch failed", err)
	}
	return nil
}

// ─────────────────────────────────────────────
// adapter.OutboundSender
// ─────────────────────────────────────────────

// Send posts a reply activity to Teams. SessionID format: teams:<serviceURL>:<conversationID>.
func (c *Channel) Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	serviceURL, convID, err := parseTeamsSession(msg.SessionID)
	if err != nil {
		return nil, err
	}
	body := outboundBody(msg)
	if body == "" {
		return nil, adapter.NewChannelError(adapter.ErrorKindPermanent, "teams: empty outbound body", nil)
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, adapter.NewChannelError(adapter.ErrorKindRetryable, "teams: token fetch", err)
	}

	// Split long messages to stay within Teams' 28 000-char limit.
	parts := splitMessage(body, 28000)
	var lastID string
	for _, part := range parts {
		id, sendErr := c.postActivity(ctx, serviceURL, convID, part, token)
		if sendErr != nil {
			return nil, sendErr
		}
		lastID = id
	}

	return &adapter.DeliveryReceipt{
		ProviderMessageID: lastID,
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            time.Now().UTC(),
	}, nil
}

// ─────────────────────────────────────────────
// adapter.InboundLifecycle
// ─────────────────────────────────────────────

// StartInbound starts the HTTP listener and dispatches inbound activities.
func (c *Channel) StartInbound(ctx context.Context, h adapter.InboundHandler) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/teams/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB cap
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		var act activity
		if err := json.Unmarshal(body, &act); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		// Only handle message activities.
		if act.Type != "message" {
			w.WriteHeader(http.StatusOK)
			return
		}
		text := strings.TrimSpace(act.Text)
		if text == "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if !c.shouldAccept(act) {
			w.WriteHeader(http.StatusOK)
			return
		}

		sessionID := teamsSessionID(act.ServiceURL, act.Conversation.ID)
		kind := "dm"
		if act.Conversation.IsGroup || act.Conversation.ConversationType == "channel" {
			kind = "group"
		}

		ev := adapter.InboundEvent{
			ID:         act.ID,
			ChannelID:  c.Name(),
			SessionID:  sessionID,
			OccurredAt: time.Now().UTC(),
			Sender: adapter.SenderRef{
				ID:          act.From.ID,
				DisplayName: act.From.Name,
			},
			Recipient: adapter.RecipientRef{
				ID:   act.Conversation.ID,
				Kind: kind,
			},
			Text: text,
			Parts: []provider.ContentPart{{
				Type: provider.ContentTypeText,
				Text: text,
			}},
			Metadata: map[string]string{
				channels.MetaTeamsConversationID: act.Conversation.ID,
				channels.MetaTeamsActivityID:     act.ID,
				channels.MetaTeamsServiceURL:     act.ServiceURL,
			},
		}

		w.WriteHeader(http.StatusOK)

		go func(ev adapter.InboundEvent) {
			if err := h(ctx, ev); err != nil {
				log.Printf("teams: inbound handler error: %v", err)
			}
		}(ev)
	})

	c.server = &http.Server{
		Addr:    c.listenAddr,
		Handler: mux,
	}

	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("teams: listener error: %v", err)
		}
	}()

	// Shut down when context is cancelled.
	go func() {
		select {
		case <-ctx.Done():
		case <-c.stopCh:
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.server.Shutdown(shutCtx)
	}()

	log.Printf("channels/teams: listening for incoming activities on %s/teams/messages", c.listenAddr)
	return nil
}

// Start implements [channels.Channel] by bridging to StartInbound.
func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	return c.StartInbound(ctx, c.legacyInboundHandler(handler))
}

func (c *Channel) legacyInboundHandler(handler channels.MessageHandler) adapter.InboundHandler {
	return func(ctx context.Context, ev adapter.InboundEvent) error {
		msg := channels.MessageFromInbound(ev)

		serviceURL := ev.Metadata[channels.MetaTeamsServiceURL]
		convID := ev.Metadata[channels.MetaTeamsConversationID]

		sendText := func(text string) error {
			token, err := c.getToken(ctx)
			if err != nil {
				return adapter.NewChannelError(adapter.ErrorKindRetryable, "teams reply: token", err)
			}
			for _, part := range splitMessage(text, 28000) {
				if c.reliableSend != nil {
					_, err = c.reliableSend.Send(ctx, adapter.OutboundMessage{
						SessionID: teamsSessionID(serviceURL, convID),
						Text:      part,
					})
				} else {
					_, err = c.postActivity(ctx, serviceURL, convID, part, token)
				}
				if err != nil {
					return err
				}
			}
			return nil
		}

		buf := channels.NewStreamingBuffer(sendText, 700*time.Millisecond)
		replyFn := func(chunk string) error {
			if chunk == "" {
				return nil
			}
			return buf.Push(chunk)
		}

		go func() {
			handler(ctx, msg, replyFn, nil, nil)
			if err := buf.Done(); err != nil {
				log.Printf("teams: flush send: %v", err)
			}
		}()
		return nil
	}
}

// Stop implements [channels.Channel] and [adapter.InboundLifecycle].
func (c *Channel) Stop() error {
	c.stopOnce.Do(func() { close(c.stopCh) })
	return nil
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func (c *Channel) shouldAccept(act activity) bool {
	if c.dmMode == "disabled" {
		return false
	}
	if c.dmMode == "allowlist" {
		senderID := act.From.ID
		allowed := false
		for _, a := range c.allowFrom {
			if senderID == a {
				allowed = true
				break
			}
		}
		if !allowed {
			log.Printf("teams: blocked message from unauthorized sender: %s", senderID)
			return false
		}
	}
	return true
}

// postActivity sends a reply activity to the Teams service URL.
func (c *Channel) postActivity(ctx context.Context, serviceURL, convID, text, token string) (string, error) {
	url := strings.TrimRight(serviceURL, "/") + "/v3/conversations/" + convID + "/activities"
	act := activity{
		Type: "message",
		Text: text,
	}
	payload, err := json.Marshal(act)
	if err != nil {
		return "", adapter.NewChannelError(adapter.ErrorKindPermanent, "teams: marshal activity", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", adapter.NewChannelError(adapter.ErrorKindPermanent, "teams: build request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", adapter.NewChannelError(adapter.ErrorKindRetryable, "teams: post activity", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", adapter.NewChannelError(adapter.ErrorKindRateLimited, "teams: rate limited", nil)
	}
	if resp.StatusCode >= 500 {
		return "", adapter.NewChannelError(adapter.ErrorKindRetryable, fmt.Sprintf("teams: server error %d", resp.StatusCode), nil)
	}
	if resp.StatusCode >= 400 {
		return "", adapter.NewChannelError(adapter.ErrorKindPermanent, fmt.Sprintf("teams: client error %d: %s", resp.StatusCode, string(respBody)), nil)
	}

	var result struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &result)
	return result.ID, nil
}

// getToken fetches (or returns a cached) Bot Framework OAuth token.
func (c *Channel) getToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.tokenValue != "" && time.Now().Before(c.tokenExp) {
		return c.tokenValue, nil
	}

	form := "grant_type=client_credentials" +
		"&client_id=" + c.appID +
		"&client_secret=" + c.appPassword +
		"&scope=https%3A%2F%2Fapi.botframework.com%2F.default"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token",
		strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("teams token: status %d: %s", resp.StatusCode, string(b))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	c.tokenValue = tok.AccessToken
	// Refresh 60 s before actual expiry.
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	return c.tokenValue, nil
}

// teamsSessionID builds the canonical session key.
// Format: teams:<conversationID>@<serviceURL>
// Using "@" as separator since Teams conversation IDs and service URLs don't contain "@".
func teamsSessionID(serviceURL, convID string) string {
	return "teams:" + convID + "@" + serviceURL
}

// parseTeamsSession splits a teams session ID back into serviceURL and conversationID.
func parseTeamsSession(sessionID string) (serviceURL, convID string, err error) {
	if !strings.HasPrefix(sessionID, "teams:") {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent,
			"teams: session id must be teams:<conversationID>@<serviceURL>", nil)
	}
	rest := strings.TrimPrefix(sessionID, "teams:")
	idx := strings.Index(rest, "@")
	if idx < 0 {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent,
			"teams: malformed session id (missing @)", nil)
	}
	return rest[idx+1:], rest[:idx], nil
}

func outboundBody(msg adapter.OutboundMessage) string {
	if msg.Text != "" {
		return msg.Text
	}
	var b strings.Builder
	for _, p := range msg.Parts {
		if p.Type == provider.ContentTypeText && p.Text != "" {
			b.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func splitMessage(s string, maxLen int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxLen <= 0 {
		return nil
	}
	var out []string
	for len(s) > maxLen {
		cut := strings.LastIndex(s[:maxLen], "\n")
		if cut < 100 {
			cut = strings.LastIndex(s[:maxLen], " ")
		}
		if cut < 100 {
			cut = maxLen
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
