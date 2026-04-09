package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// Compile-time checks: enterprise adapter v1 + legacy channel.
var (
	_ adapter.Adapter  = (*Channel)(nil)
	_ channels.Channel = (*Channel)(nil)
)

// Channel implements [channels.Channel] and [adapter.Adapter] for Telegram.
type Channel struct {
	bot            *tgbotapi.BotAPI
	stopCh         chan struct{}
	stopOnce       sync.Once
	dmMode         string
	allowFrom      []string
	requireMention bool
	reliableSend   *adapter.ReliableSender
}

func New(apiKey string, dmMode string, allowFrom []string, requireMention bool) (*Channel, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("telegram API key is required")
	}
	bot, err := tgbotapi.NewBotAPI(apiKey)
	if err != nil {
		return nil, err
	}
	return &Channel{
		bot:            bot,
		stopCh:         make(chan struct{}),
		dmMode:         dmMode,
		allowFrom:      allowFrom,
		requireMention: requireMention,
	}, nil
}

// WithReliableOutbound sets the shared reliability wrapper used for outbound
// sends from Telegram legacy reply paths.
func (c *Channel) WithReliableOutbound(rs *adapter.ReliableSender) *Channel {
	c.reliableSend = rs
	return c
}

// Name implements [channels.Channel] and [adapter.Identity].
func (c *Channel) Name() string {
	return "telegram"
}

// AdapterVersion implements [adapter.Identity].
func (c *Channel) AdapterVersion() int {
	return adapter.Version1
}

// Capabilities implements [adapter.Identity].
func (c *Channel) Capabilities() adapter.ChannelCapabilities {
	return adapter.ChannelCapabilities{
		Threading:        true,
		Attachments:      true,
		DirectMessages:   true,
		GroupMessages:    true,
		Mentions:         true,
		Voice:            false,
		Reactions:        true,
		Edits:            true,
		MaxMessageLength: 4096,
	}
}

// Ping implements [adapter.Health] (Bot API GetMe).
func (c *Channel) Ping(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { _, err := c.bot.GetMe(); done <- err }()
	select {
	case <-ctx.Done():
		return adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram ping cancelled", ctx.Err())
	case err := <-done:
		if err != nil {
			return adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram getMe failed", err)
		}
		return nil
	}
}

// Send implements [adapter.OutboundSender] for proactive outbound (cron, tools) using
// session id tg:<chatID> or tg:<chatID>:<threadID>.
func (c *Channel) Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	target, err := parseTelegramSession(msg.SessionID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram send cancelled", err)
	}
	body := outboundBody(msg)
	if body == "" {
		return nil, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: empty outbound body", nil)
	}
	var sent tgbotapi.Message
	for i, part := range splitTelegramMessage(body) {
		m := tgbotapi.NewMessage(target.ChatID, part)
		if i == 0 {
			if msg.ReplyToID != "" {
				if replyID, convErr := strconv.Atoi(msg.ReplyToID); convErr == nil {
					m.ReplyToMessageID = replyID
				}
			}
			if msg.ThreadRef != nil && msg.ThreadRef.ParentMessageID != "" && m.ReplyToMessageID == 0 {
				if replyID, convErr := strconv.Atoi(msg.ThreadRef.ParentMessageID); convErr == nil {
					m.ReplyToMessageID = replyID
				}
			}
		}
		out, sendErr := c.bot.Send(m)
		if sendErr != nil {
			return nil, classifyTelegramSendErr(sendErr)
		}
		if i == 0 {
			sent = out
		}
	}
	now := time.Now().UTC()
	return &adapter.DeliveryReceipt{
		ProviderMessageID: strconv.Itoa(sent.MessageID),
		IdempotencyKey:    msg.IdempotencyKey,
		SentAt:            now,
	}, nil
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

type telegramTarget struct {
	ChatID   int64
	ThreadID int
}

func parseTelegramSession(sessionID string) (telegramTarget, error) {
	const prefix = "tg:"
	if !strings.HasPrefix(sessionID, prefix) {
		return telegramTarget{}, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: session id must be tg:<chatId>[:threadId]", nil)
	}
	raw := strings.TrimPrefix(sessionID, prefix)
	parts := strings.Split(raw, ":")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id == 0 {
		return telegramTarget{}, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: invalid chat id in session", err)
	}
	target := telegramTarget{ChatID: id}
	if len(parts) >= 2 && parts[1] != "" {
		threadID, convErr := strconv.Atoi(parts[1])
		if convErr != nil || threadID <= 0 {
			return telegramTarget{}, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: invalid thread id in session", convErr)
		}
		target.ThreadID = threadID
	}
	return target, nil
}

func classifyTelegramSendErr(err error) error {
	if err == nil {
		return nil
	}
	// Telegram often returns retryable network errors; treat unknown as retryable for reliability layers.
	return adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram send", err)
}

// StartInbound implements [adapter.InboundLifecycle]: normalized inbound events.
func (c *Channel) StartInbound(ctx context.Context, h adapter.InboundHandler) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := c.bot.GetUpdatesChan(u)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case update := <-updates:
				if update.Message == nil || update.Message.Text == "" {
					continue
				}
				if !c.shouldAcceptMessage(update.Message) {
					continue
				}

				chatID := update.Message.Chat.ID
				sessionID := fmt.Sprintf("tg:%d", chatID)
				senderID := fmt.Sprintf("%d", update.Message.From.ID)
				username := update.Message.From.UserName

				if c.dmMode == "disabled" {
					continue
				}

				if c.dmMode == "allowlist" {
					allowed := false
					for _, allowedNum := range c.allowFrom {
						if senderID == allowedNum || (username != "" && username == strings.TrimPrefix(allowedNum, "@")) {
							allowed = true
							break
						}
					}
					if !allowed {
						log.Printf("Telegram: blocked message from unauthorized sender: %s (@%s)", senderID, username)
						continue
					}
				}

				kind := "dm"
				if update.Message.Chat.Type == "group" || update.Message.Chat.Type == "supergroup" {
					kind = "group"
				}

				ev := adapter.InboundEvent{
					ID:         strconv.FormatInt(int64(update.Message.MessageID), 10),
					ChannelID:  c.Name(),
					SessionID:  sessionID,
					OccurredAt: time.Unix(int64(update.Message.Date), 0).UTC(),
					Sender: adapter.SenderRef{
						ID:          senderID,
						DisplayName: strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName),
						Username:    username,
					},
					Recipient: adapter.RecipientRef{
						ID:   fmt.Sprintf("%d", chatID),
						Kind: kind,
					},
					Text: update.Message.Text,
					Parts: []provider.ContentPart{{
						Type: provider.ContentTypeText,
						Text: update.Message.Text,
					}},
					Metadata: map[string]string{
						channels.MetaTelegramChatID:    strconv.FormatInt(chatID, 10),
						channels.MetaTelegramMessageID: strconv.Itoa(update.Message.MessageID),
					},
				}
				go func(ev adapter.InboundEvent) {
					if err := h(ctx, ev); err != nil {
						log.Printf("Telegram: inbound handler error: %v", err)
					}
				}(ev)
			}
		}
	}()

	log.Printf("channels/telegram: listening for incoming messages on %s", c.bot.Self.UserName)
	return nil
}

// Start implements [channels.Channel] by bridging to [Channel.StartInbound] with legacy [channels.MessageHandler] semantics.
func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	return c.StartInbound(ctx, c.legacyInboundHandler(handler))
}

func (c *Channel) legacyInboundHandler(handler channels.MessageHandler) adapter.InboundHandler {
	return func(ctx context.Context, ev adapter.InboundEvent) error {
		chatIDStr := ev.Metadata[channels.MetaTelegramChatID]
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			return adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: missing or invalid chat id", err)
		}

		if _, err := c.bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
			log.Printf("Telegram: chat action: %v", err)
		}

		msg := channels.MessageFromInbound(ev)

		replyToID := 0
		if msgID := ev.Metadata[channels.MetaTelegramMessageID]; msgID != "" {
			if parsed, convErr := strconv.Atoi(msgID); convErr == nil && parsed > 0 {
				replyToID = parsed
			}
		}

		sendText := func(text string) error {
			for i, part := range splitTelegramMessage(text) {
				out := adapter.OutboundMessage{
					SessionID: fmt.Sprintf("tg:%d", chatID),
					Text:      part,
				}
				if i == 0 && replyToID > 0 {
					out.ReplyToID = strconv.Itoa(replyToID)
				}
				if c.reliableSend != nil {
					if _, err := c.reliableSend.Send(ctx, out); err != nil {
						return adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram reply send", err)
					}
					continue
				}
				if _, err := c.Send(ctx, out); err != nil {
					return adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram reply send", err)
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
				log.Printf("Telegram: flush send: %v", err)
			}
		}()
		return nil
	}
}

func splitTelegramMessage(s string) []string {
	const maxLen = 4096
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > maxLen {
		cut := strings.LastIndex(s[:maxLen], "\n")
		if cut < 120 {
			cut = strings.LastIndex(s[:maxLen], " ")
		}
		if cut < 120 {
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

func (c *Channel) shouldAcceptMessage(m *tgbotapi.Message) bool {
	if m.Chat == nil {
		return false
	}
	chatType := m.Chat.Type
	if chatType != "group" && chatType != "supergroup" {
		return true
	}
	if !c.requireMention {
		return true
	}
	username := strings.ToLower(strings.TrimSpace(c.bot.Self.UserName))
	if username == "" {
		return true
	}
	// If user replies directly to the bot's prior message in group, allow.
	if m.ReplyToMessage != nil && m.ReplyToMessage.From != nil {
		if strings.EqualFold(m.ReplyToMessage.From.UserName, c.bot.Self.UserName) {
			return true
		}
	}
	text := strings.ToLower(m.Text)
	needle := "@" + username
	if strings.Contains(text, needle) {
		return true
	}
	return false
}

// Stop implements [channels.Channel] and [adapter.InboundLifecycle].
func (c *Channel) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.bot.StopReceivingUpdates()
	})
	return nil
}
