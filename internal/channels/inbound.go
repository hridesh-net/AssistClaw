package channels

import (
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
)

// Metadata keys for [adapter.InboundEvent.Metadata] string maps. Used by bridges, doctor, and outbound Send.
const (
	MetaTelegramChatID    = "telegram_chat_id"
	MetaTelegramThreadID  = "telegram_thread_id"
	MetaTelegramMessageID = "telegram_message_id"

	MetaDiscordChannelID = "discord_channel_id"
	MetaDiscordGuildID   = "discord_guild_id"

	MetaSlackChannelID = "slack_channel_id"
	MetaSlackThreadTS  = "slack_thread_ts"

	MetaTeamsConversationID = "teams_conversation_id"
	MetaTeamsActivityID     = "teams_activity_id"
	MetaTeamsServiceURL     = "teams_service_url"
)

// MessageFromInbound maps a normalized adapter event to the legacy [Message] shape for [MessageHandler].
func MessageFromInbound(ev adapter.InboundEvent) Message {
	meta := make(map[string]any, len(ev.Metadata)+1)
	for k, v := range ev.Metadata {
		meta[k] = v
	}
	return Message{
		ID:        ev.ID,
		ChannelID: ev.ChannelID,
		SessionID: ev.SessionID,
		Text:      ev.Text,
		Parts:     ev.Parts,
		Metadata:  meta,
	}
}
