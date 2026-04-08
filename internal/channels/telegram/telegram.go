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
	bot       *tgbotapi.BotAPI
	stopCh    chan struct{}
	stopOnce  sync.Once
	dmMode    string
	allowFrom []string
}

func New(apiKey string, dmMode string, allowFrom []string) (*Channel, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("telegram API key is required")
	}
	bot, err := tgbotapi.NewBotAPI(apiKey)
	if err != nil {
		return nil, err
	}
	return &Channel{
		bot:       bot,
		stopCh:    make(chan struct{}),
		dmMode:    dmMode,
		allowFrom: allowFrom,
	}, nil
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

// Send implements [adapter.OutboundSender] for proactive outbound (cron, tools) using session id tg:<chatID>.
func (c *Channel) Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	chatID, err := parseTelegramSession(msg.SessionID)
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
	m := tgbotapi.NewMessage(chatID, body)
	sent, err := c.bot.Send(m)
	if err != nil {
		return nil, classifyTelegramSendErr(err)
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

func parseTelegramSession(sessionID string) (int64, error) {
	const prefix = "tg:"
	if !strings.HasPrefix(sessionID, prefix) {
		return 0, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: session id must be tg:<chatId>", nil)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sessionID, prefix), 10, 64)
	if err != nil || id == 0 {
		return 0, adapter.NewChannelError(adapter.ErrorKindPermanent, "telegram: invalid chat id in session", err)
	}
	return id, nil
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
						channels.MetaTelegramChatID: strconv.FormatInt(chatID, 10),
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

		var buffer string
		replyFn := func(chunk string) error {
			buffer += chunk
			if chunk == "" || len(buffer) > 200 {
				if len(buffer) > 0 {
					tgMsg := tgbotapi.NewMessage(chatID, buffer)
					if _, err := c.bot.Send(tgMsg); err != nil {
						return adapter.NewChannelError(adapter.ErrorKindRetryable, "telegram reply send", err)
					}
					buffer = ""
				}
			}
			return nil
		}

		go func() {
			handler(ctx, msg, replyFn, nil, nil)
			if buffer != "" {
				tgMsg := tgbotapi.NewMessage(chatID, buffer)
				if _, err := c.bot.Send(tgMsg); err != nil {
					log.Printf("Telegram: flush send: %v", err)
				}
			}
		}()
		return nil
	}
}

// Stop implements [channels.Channel] and [adapter.InboundLifecycle].
func (c *Channel) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.bot.StopReceivingUpdates()
	})
	return nil
}
