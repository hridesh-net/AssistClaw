package proactive

import (
	"context"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
)

// ChannelAdapter is the minimal interface needed from a channel to send notifications.
type ChannelAdapter interface {
	Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error)
}

// TelegramNotifier sends proactive notifications via Telegram.
type TelegramNotifier struct {
	channel    ChannelAdapter
	sessionID  string // e.g. "tg:123456789"
}

// NewTelegramNotifier creates a Telegram notifier backed by an existing channel adapter.
func NewTelegramNotifier(channel ChannelAdapter, sessionID string) *TelegramNotifier {
	return &TelegramNotifier{channel: channel, sessionID: sessionID}
}

// Name returns "telegram".
func (n *TelegramNotifier) Name() string { return "telegram" }

// Send delivers the notification body to the configured Telegram chat.
func (n *TelegramNotifier) Send(ctx context.Context, notif Notification) error {
	_, err := n.channel.Send(ctx, adapter.OutboundMessage{
		SessionID: n.sessionID,
		Text:      fmt.Sprintf("🤖 *Proactive* — %s\n\n%s", notif.RuleID, notif.Body),
	})
	return err
}

// DiscordNotifier sends proactive notifications via Discord.
type DiscordNotifier struct {
	channel   ChannelAdapter
	sessionID string // e.g. "discord:guildID:channelID"
}

// NewDiscordNotifier creates a Discord notifier backed by an existing channel adapter.
func NewDiscordNotifier(channel ChannelAdapter, sessionID string) *DiscordNotifier {
	return &DiscordNotifier{channel: channel, sessionID: sessionID}
}

// Name returns "discord".
func (n *DiscordNotifier) Name() string { return "discord" }

// Send delivers the notification body to the configured Discord channel.
func (n *DiscordNotifier) Send(ctx context.Context, notif Notification) error {
	_, err := n.channel.Send(ctx, adapter.OutboundMessage{
		SessionID: n.sessionID,
		Text:      fmt.Sprintf("🤖 **Proactive** — %s\n\n%s", notif.RuleID, notif.Body),
	})
	return err
}
