package discord

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/voice"
)

// Compile-time checks.
var (
	_ adapter.Adapter  = (*Channel)(nil)
	_ channels.Channel = (*Channel)(nil)
)

// Channel implements [channels.Channel] and [adapter.Adapter] for Discord.
type Channel struct {
	session          *discordgo.Session
	dmMode           string
	allowFrom        []string
	requireMention   bool
	reliableSend     *adapter.ReliableSender
	voiceClient      *voice.Client
	voiceConnections map[string]*discordgo.VoiceConnection

	legacyMu      sync.RWMutex
	legacyHandler channels.MessageHandler // used by voice path (!join) until refactored to InboundEvent
	// sem bounds concurrent inbound handler goroutines to prevent bursts from
	// ballooning goroutine count.
	sem      chan struct{}
	voiceSem chan struct{}
}

func New(token string, dmMode string, allowFrom []string, requireMention bool, voiceClient *voice.Client) (*Channel, error) {
	if token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}
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
		requireMention:   requireMention,
		voiceClient:      voiceClient,
		voiceConnections: make(map[string]*discordgo.VoiceConnection),
		sem:              make(chan struct{}, 50),
		voiceSem:         make(chan struct{}, 5),
	}, nil
}

// WithReliableOutbound wires shared adapter reliability for Discord reply sends.
func (c *Channel) WithReliableOutbound(rs *adapter.ReliableSender) *Channel {
	c.reliableSend = rs
	return c
}

func (c *Channel) Name() string {
	return "discord"
}

func (c *Channel) AdapterVersion() int {
	return adapter.Version1
}

func (c *Channel) Capabilities() adapter.ChannelCapabilities {
	caps, ok := adapter.BuiltinCaps(c.Name())
	if !ok {
		return adapter.ChannelCapabilities{}
	}
	return caps
}

// Ping verifies the bot token via GET /users/@me.
func (c *Channel) Ping(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		_, err := c.session.User("@me")
		done <- err
	}()
	select {
	case <-ctx.Done():
		return adapter.NewChannelError(adapter.ErrorKindRetryable, "discord ping cancelled", ctx.Err())
	case err := <-done:
		if err != nil {
			return adapter.NewChannelError(adapter.ErrorKindPermanent, "discord @me failed", err)
		}
		return nil
	}
}

// Send posts a message to a text channel. Session id format: discord:<guildID>:<channelID> (guild may be empty for DMs, e.g. discord::channelID).
func (c *Channel) Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	guildID, channelID, err := parseDiscordSession(msg.SessionID)
	if err != nil {
		return nil, err
	}
	_ = guildID // channel send uses channel id only
	body := outboundBody(msg)
	if body == "" {
		return nil, adapter.NewChannelError(adapter.ErrorKindPermanent, "discord: empty outbound body", nil)
	}
	caps, _ := adapter.BuiltinCaps(c.Name())
	send := &discordgo.MessageSend{Content: body}
	if caps.InteractiveButtons && len(msg.InlineKeyboard) > 0 {
		for _, row := range msg.InlineKeyboard {
			if len(row) == 0 {
				continue
			}
			ar := &discordgo.ActionsRow{}
			for _, b := range row {
				style := discordgo.PrimaryButton
				low := strings.ToLower(strings.TrimSpace(b.CallbackData))
				switch {
				case strings.HasPrefix(low, "approve"):
					style = discordgo.SuccessButton
				case strings.HasPrefix(low, "reject"):
					style = discordgo.DangerButton
				}
				cid := strings.TrimSpace(b.CallbackData)
				if len(cid) > 100 {
					cid = cid[:100]
				}
				lbl := strings.TrimSpace(b.Label)
				if lbl == "" {
					lbl = "…"
				}
				lbl = truncateDiscordRunes(lbl, 78)
				ar.Components = append(ar.Components, &discordgo.Button{
					Label:    lbl,
					Style:    style,
					CustomID: cid,
				})
			}
			if len(ar.Components) > 0 {
				send.Components = append(send.Components, ar)
			}
		}
		if len(send.Components) == 0 {
			send.Components = nil
		}
	}
	sent, err := c.session.ChannelMessageSendComplex(channelID, send)
	if err != nil {
		return nil, adapter.NewChannelError(adapter.ErrorKindRetryable, "discord send", err)
	}
	now := time.Now().UTC()
	return &adapter.DeliveryReceipt{
		ProviderMessageID: sent.ID,
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

func parseDiscordSession(sessionID string) (guildID, channelID string, err error) {
	if !strings.HasPrefix(sessionID, "discord:") {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent, "discord: invalid session prefix", nil)
	}
	rest := strings.TrimPrefix(sessionID, "discord:")
	if strings.HasPrefix(rest, "voice:") {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent, "discord: outbound send not supported for voice sessions", nil)
	}
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent, "discord: invalid session id", nil)
	}
	return parts[0], parts[1], nil
}

func (c *Channel) listenVoice(ctx context.Context, vc *discordgo.VoiceConnection, guildID, channelID string, handler channels.MessageHandler) {
	log.Printf("Discord: listening to voice in guild %s", guildID)

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
			audioBuffer.Write(p.Opus)
		case <-ticker.C:
			if audioBuffer.Len() > 0 {
				data := audioBuffer.Bytes()
				audioBuffer.Reset()

				select {
				case c.voiceSem <- struct{}{}:
					go func(d []byte) {
						defer func() { <-c.voiceSem }()
						text, err := c.voiceClient.STT(d, "opus")
						if err == nil && text != "" {
							msg := channels.Message{
								ChannelID: c.Name(),
								SessionID: "discord:voice:" + guildID + ":" + channelID,
								Text:      text,
							}

							replyFn := func(chunk string) error {
								return c.speakVoice(vc, chunk)
							}

							handler(ctx, msg, replyFn, nil, nil)
						}
					}(data)
				default:
					// Drop audio chunk if voice pipeline is saturated.
				}
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

	vc.Speaking(true)
	defer vc.Speaking(false)

	for _, p := range packets {
		vc.OpusSend <- p
	}

	return nil
}

// StartInbound implements [adapter.InboundLifecycle].
func (c *Channel) StartInbound(ctx context.Context, h adapter.InboundHandler) error {
	c.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		if !c.shouldAcceptMessage(m) {
			return
		}

		sessionID := "discord:" + m.GuildID + ":" + m.ChannelID
		authorID := m.Author.ID

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

		if strings.HasPrefix(m.Content, "!join") {
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

			hFn := c.getLegacyHandler()
			if hFn == nil {
				log.Printf("Discord: voice join but legacy handler not set")
				return
			}
			go c.listenVoice(ctx, vc, m.GuildID, m.ChannelID, hFn)
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

		ev := adapter.InboundEvent{
			ID:         m.ID,
			ChannelID:  c.Name(),
			SessionID:  sessionID,
			OccurredAt: m.Timestamp,
			Sender: adapter.SenderRef{
				ID:          m.Author.ID,
				DisplayName: m.Author.Username,
				Username:    m.Author.Username,
			},
			Recipient: adapter.RecipientRef{
				ID:   m.ChannelID,
				Kind: "channel",
			},
			Text: m.Content,
			Parts: []provider.ContentPart{{
				Type: provider.ContentTypeText,
				Text: m.Content,
			}},
			Metadata: map[string]string{
				channels.MetaDiscordChannelID: m.ChannelID,
				channels.MetaDiscordGuildID:   m.GuildID,
			},
		}

		c.sem <- struct{}{}
		go func() {
			defer func() { <-c.sem }()
			if err := h(ctx, ev); err != nil {
				log.Printf("Discord: inbound handler error: %v", err)
			}
		}()
	})

	c.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i == nil || i.Interaction == nil || i.Type != discordgo.InteractionMessageComponent {
			return
		}
		data := i.MessageComponentData()
		cb := strings.TrimSpace(data.CustomID)
		if cb == "" {
			return
		}
		low := strings.ToLower(cb)
		if !strings.HasPrefix(low, "approve ") && !strings.HasPrefix(low, "reject ") {
			return
		}
		uid := ""
		if i.Member != nil && i.Member.User != nil {
			uid = i.Member.User.ID
		} else if i.User != nil {
			uid = i.User.ID
		}
		if uid == "" {
			return
		}
		if c.dmMode == "disabled" {
			return
		}
		if c.dmMode == "allowlist" {
			allowed := false
			for _, allowedNum := range c.allowFrom {
				if uid == allowedNum {
					allowed = true
					break
				}
			}
			if !allowed {
				log.Printf("Discord: blocked component interaction from unauthorized sender: %s", uid)
				return
			}
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "\u200b",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		}); err != nil {
			log.Printf("Discord: interaction respond: %v", err)
			return
		}
		sessionID := "discord:" + i.GuildID + ":" + i.ChannelID
		ev := adapter.InboundEvent{
			ID:         i.ID,
			ChannelID:  c.Name(),
			SessionID:  sessionID,
			OccurredAt: time.Now().UTC(),
			Sender: adapter.SenderRef{
				ID:          uid,
				DisplayName: "",
				Username:    "",
			},
			Recipient: adapter.RecipientRef{
				ID:   i.ChannelID,
				Kind: "channel",
			},
			Text: cb,
			Parts: []provider.ContentPart{{
				Type: provider.ContentTypeText,
				Text: cb,
			}},
			Metadata: map[string]string{
				channels.MetaDiscordChannelID: i.ChannelID,
				channels.MetaDiscordGuildID:   i.GuildID,
			},
		}
		c.sem <- struct{}{}
		go func(ev adapter.InboundEvent) {
			defer func() { <-c.sem }()
			if err := h(ctx, ev); err != nil {
				log.Printf("Discord: inbound handler error: %v", err)
			}
		}(ev)
	})

	err := c.session.Open()
	if err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	log.Printf("channels/discord: listening for incoming messages")
	return nil
}

func (c *Channel) setLegacyHandler(h channels.MessageHandler) {
	c.legacyMu.Lock()
	c.legacyHandler = h
	c.legacyMu.Unlock()
}

func (c *Channel) getLegacyHandler() channels.MessageHandler {
	c.legacyMu.RLock()
	defer c.legacyMu.RUnlock()
	return c.legacyHandler
}

// Start implements [channels.Channel].
func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	c.setLegacyHandler(handler)
	return c.StartInbound(ctx, c.legacyInboundHandler(handler))
}

func (c *Channel) legacyInboundHandler(handler channels.MessageHandler) adapter.InboundHandler {
	return func(ctx context.Context, ev adapter.InboundEvent) error {
		chID := ev.Metadata[channels.MetaDiscordChannelID]
		if chID == "" {
			return adapter.NewChannelError(adapter.ErrorKindPermanent, "discord: missing channel id", nil)
		}
		guildID := ev.Metadata[channels.MetaDiscordGuildID]

		msg := channels.MessageFromInbound(ev)

		sendText := func(text string) error {
			for _, part := range splitDiscordMessage(text) {
				out := adapter.OutboundMessage{
					SessionID: fmt.Sprintf("discord:%s:%s", guildID, chID),
					Text:      part,
				}
				if c.reliableSend != nil {
					if _, err := c.reliableSend.Send(ctx, out); err != nil {
						return adapter.NewChannelError(adapter.ErrorKindRetryable, "discord reply send", err)
					}
				} else {
					if _, err := c.Send(ctx, out); err != nil {
						return adapter.NewChannelError(adapter.ErrorKindRetryable, "discord reply send", err)
					}
				}
				if vc, ok := c.voiceConnections[guildID]; ok {
					go c.speakVoice(vc, part)
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

		c.sem <- struct{}{}
		go func() {
			defer func() { <-c.sem }()
			handler(ctx, msg, replyFn, nil, nil)
			if err := buf.Done(); err != nil {
				log.Printf("Discord: flush send: %v", err)
			}
		}()
		return nil
	}
}

func truncateDiscordRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n >= maxRunes {
			return strings.TrimSpace(s[:i])
		}
		n++
	}
	return s
}

func splitDiscordMessage(s string) []string {
	const maxLen = 2000
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

func (c *Channel) shouldAcceptMessage(m *discordgo.MessageCreate) bool {
	if m == nil || m.Message == nil || m.Author == nil || m.GuildID == "" {
		return true // DMs and malformed events pass through existing policies.
	}
	if !c.requireMention {
		return true
	}
	if c.session == nil || c.session.State == nil || c.session.State.User == nil {
		return true
	}
	botID := c.session.State.User.ID
	if botID == "" {
		return true
	}
	if strings.Contains(m.Content, "<@"+botID+">") || strings.Contains(m.Content, "<@!"+botID+">") {
		return true
	}
	if m.MessageReference != nil && m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		if m.ReferencedMessage.Author.ID == botID {
			return true
		}
	}
	return false
}

// Stop implements [channels.Channel] and [adapter.InboundLifecycle].
func (c *Channel) Stop() error {
	c.setLegacyHandler(nil)
	return c.session.Close()
}
