package discord

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/voice"
)

// Channel implements channels.Channel for Discord
type Channel struct {
	session          *discordgo.Session
	dmMode           string
	allowFrom        []string
	voiceClient      *voice.Client
	voiceConnections map[string]*discordgo.VoiceConnection
}

func New(token string, dmMode string, allowFrom []string, voiceClient *voice.Client) (*Channel, error) {
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
	return &Channel{
		session:          dg,
		dmMode:           dmMode,
		allowFrom:        allowFrom,
		voiceClient:      voiceClient,
		voiceConnections: make(map[string]*discordgo.VoiceConnection),
	}, nil
}

func (c *Channel) listenVoice(ctx context.Context, vc *discordgo.VoiceConnection, guildID, channelID string, handler channels.MessageHandler) {
	log.Printf("Discord: listening to voice in guild %s", guildID)
	
	// We'll use a simple buffer for now. 
	// In a real impl, we'd use SAD.
	// For this prototype, we'll collect 5 seconds of audio and send it.
	
	audioBuffer := bytes.Buffer{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}
			// We receive Opus packets. 
			// We'll send them to the voice server which can decode them.
			audioBuffer.Write(p.Opus)
		case <-ticker.C:
			if audioBuffer.Len() > 0 {
				data := audioBuffer.Bytes()
				audioBuffer.Reset()
				
				go func(d []byte) {
					text, err := c.voiceClient.STT(d)
					if err == nil && text != "" {
						msg := channels.Message{
							ChannelID: c.Name(),
							SessionID: fmt.Sprintf("discord:voice:%s:%s", guildID, channelID),
							Text:      text,
						}
						
						replyFn := func(chunk string) error {
							return c.speakVoice(vc, chunk)
						}
						
						handler(ctx, msg, replyFn, nil, nil)
					}
				}(data)
			}
		}
	}
}

func (c *Channel) speakVoice(vc *discordgo.VoiceConnection, text string) error {
	if c.voiceClient == nil {
		return fmt.Errorf("voice client not initialized")
	}
	
	packets, err := c.voiceClient.TTSDiscord(text)
	if err != nil {
		return err
	}
	
	log.Printf("Discord: speaking '%s' (%d packets)", text, len(packets))
	
	// Send speaking status
	vc.Speaking(true)
	defer vc.Speaking(false)

	for _, p := range packets {
		vc.OpusSend <- p
	}
	
	return nil
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
		authorID := m.Author.ID

		// Enforce security policies
		if c.dmMode == "disabled" {
			return
		}

		if c.dmMode == "allowlist" {
			allowed := false
			for _, allowedNum := range c.allowFrom {
				if authorID == allowedNum {
					allowed = true
					break
				}
			}
			if !allowed {
				log.Printf("Discord: blocked message from unauthorized sender: %s", authorID)
				return
			}
		}

		// Handle Voice Channel Commands
		if strings.HasPrefix(m.Content, "!join") {
			// Find voice channel of the user
			vs, err := s.State.VoiceState(m.GuildID, m.Author.ID)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "You must be in a voice channel!")
				return
			}
			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "Could not join voice channel: "+err.Error())
				return
			}
			c.voiceConnections[m.GuildID] = vc
			s.ChannelMessageSend(m.ChannelID, "Joined your voice channel!")
			
			// Start listening in background
			go c.listenVoice(ctx, vc, m.GuildID, m.ChannelID, handler)
			return
		}

		if strings.HasPrefix(m.Content, "!leave") {
			if vc, ok := c.voiceConnections[m.GuildID]; ok {
				vc.Disconnect()
				delete(c.voiceConnections, m.GuildID)
				s.ChannelMessageSend(m.ChannelID, "Left voice channel.")
			}
			return
		}

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
					// Send text reply
					s.ChannelMessageSend(m.ChannelID, buffer)
					
					// Also speak if we have a voice connection for this guild
					if vc, ok := c.voiceConnections[m.GuildID]; ok {
						go c.speakVoice(vc, buffer)
					}
					
					buffer = ""
				}
			}
			return nil
		}

		go func() {
			handler(ctx, msg, replyFn, nil, nil)
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
