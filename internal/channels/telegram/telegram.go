package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/assistclaw/assistclaw/internal/channels"
)

// Channel implements channels.Channel for Telegram
type Channel struct {
	bot       *tgbotapi.BotAPI
	stopCh    chan struct{}
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

func (c *Channel) Name() string {
	return "telegram"
}

func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
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

				// Enforce security policies
				if c.dmMode == "disabled" {
					continue
				}

				if c.dmMode == "allowlist" {
					allowed := false
					for _, allowedNum := range c.allowFrom {
						// Check if sender ID or username matches
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

				// Send typing indicator
				c.bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))

				msg := channels.Message{
					ChannelID: c.Name(),
					SessionID: sessionID,
					Text:      update.Message.Text,
				}

				// Simple buffer for "streaming" (Telegram prefers batched edits over token-by-token)
				// For a full implementation, this reply func should edit the same message periodically.
				var buffer string
				replyFn := func(chunk string) error {
					buffer += chunk
					// Flush on newlines or done, simplified for now:
					if chunk == "" || len(buffer) > 200 {
						if len(buffer) > 0 {
							tgMsg := tgbotapi.NewMessage(chatID, buffer)
							c.bot.Send(tgMsg)
							buffer = ""
						}
					}
					return nil
				}

				// Process message async
				go func() {
					handler(ctx, msg, replyFn, nil, nil) // Reactions & Media not yet supported for TG
					// Flush remaining buffer
					if buffer != "" {
						tgMsg := tgbotapi.NewMessage(chatID, buffer)
						c.bot.Send(tgMsg)
					}
				}()
			}
		}
	}()

	log.Printf("channels/telegram: listening for incoming messages on %s", c.bot.Self.UserName)
	return nil
}

func (c *Channel) Stop() error {
	close(c.stopCh)
	c.bot.StopReceivingUpdates()
	return nil
}
