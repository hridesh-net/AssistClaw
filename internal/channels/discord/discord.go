package discord

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/assistclaw/assistclaw/internal/channels"
)

// Channel implements channels.Channel for Discord
type Channel struct {
	session *discordgo.Session
}

func New(token string) (*Channel, error) {
	if token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}
	// Discord tokens must have the "Bot " prefix if it's a bot account
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}
	dg, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}
	return &Channel{session: dg}, nil
}

func (c *Channel) Name() string {
	return "discord"
}

func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	c.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore bot's own messages
		if m.Author.ID == s.State.User.ID {
			return
		}

		sessionID := fmt.Sprintf("discord:%s:%s", m.GuildID, m.ChannelID)

		s.ChannelTyping(m.ChannelID)

		msg := channels.Message{
			ChannelID: c.Name(),
			SessionID: sessionID,
			Text:      m.Content,
		}

		var buffer string
		replyFn := func(chunk string) error {
			buffer += chunk
			if chunk == "" || len(buffer) > 500 {
				if len(buffer) > 0 {
					s.ChannelMessageSend(m.ChannelID, buffer)
					buffer = ""
				}
			}
			return nil
		}

		go func() {
			handler(ctx, msg, replyFn)
			if buffer != "" {
				s.ChannelMessageSend(m.ChannelID, buffer)
			}
		}()
	})

	err := c.session.Open()
	if err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	log.Printf("channels/discord: listening for incoming messages")
	return nil
}

func (c *Channel) Stop() error {
	return c.session.Close()
}
