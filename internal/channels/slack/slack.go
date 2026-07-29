package slack

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// Compile-time checks.
var (
	_ adapter.Adapter  = (*Channel)(nil)
	_ channels.Channel = (*Channel)(nil)
)

// Channel implements [channels.Channel] and [adapter.Adapter] for Slack Socket Mode.
type Channel struct {
	client       *socketmode.Client
	stopCh       chan struct{}
	stopOnce     sync.Once
	dmMode       string
	allowFrom    []string
	reliableSend *adapter.ReliableSender
	// sem bounds concurrent inbound handler goroutines to prevent bursts from
	// ballooning goroutine count.
	sem chan struct{}
}

func New(botToken, appToken string, dmMode string, allowFrom []string) (*Channel, error) {
	if botToken == "" || appToken == "" {
		return nil, fmt.Errorf("slack bot token and app token are required")
	}

	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	client := socketmode.New(api)

	return &Channel{
		client:    client,
		stopCh:    make(chan struct{}),
		dmMode:    dmMode,
		allowFrom: allowFrom,
		sem:       make(chan struct{}, 50),
	}, nil
}

func (c *Channel) Name() string {
	return "slack"
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

// WithReliableOutbound wires shared adapter reliability for Slack reply sends.
func (c *Channel) WithReliableOutbound(rs *adapter.ReliableSender) *Channel {
	c.reliableSend = rs
	return c
}

// Ping calls auth.test (embedded [slack.Client]).
func (c *Channel) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := c.client.AuthTestContext(ctx)
	if err != nil {
		return adapter.NewChannelError(adapter.ErrorKindPermanent, "slack auth.test failed", err)
	}
	return nil
}

// Send posts to a channel; session id is slack:<channelID> or slack:<channelID>:<thread_ts>.
func (c *Channel) Send(ctx context.Context, msg adapter.OutboundMessage) (*adapter.DeliveryReceipt, error) {
	chID, threadTS, err := parseSlackSession(msg.SessionID)
	if err != nil {
		return nil, err
	}
	body := outboundBody(msg)
	if body == "" {
		return nil, adapter.NewChannelError(adapter.ErrorKindPermanent, "slack: empty outbound body", nil)
	}
	caps, _ := adapter.BuiltinCaps(c.Name())
	var opts []slack.MsgOption
	if caps.InteractiveButtons && len(msg.InlineKeyboard) > 0 {
		blocks := slackBlocksFromMail(body, msg.InlineKeyboard)
		opts = append(opts, slack.MsgOptionBlocks(blocks.BlockSet...))
	} else {
		opts = append(opts, slack.MsgOptionText(body, false))
	}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}
	_, ts, err := c.client.PostMessageContext(ctx, chID, opts...)
	if err != nil {
		return nil, adapter.NewChannelError(adapter.ErrorKindRetryable, "slack post message", err)
	}
	now := time.Now().UTC()
	return &adapter.DeliveryReceipt{
		ProviderMessageID: ts,
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

func parseSlackSession(sessionID string) (channelID, threadTS string, err error) {
	if !strings.HasPrefix(sessionID, "slack:") {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent, "slack: invalid session prefix", nil)
	}
	rest := strings.TrimPrefix(sessionID, "slack:")
	if rest == "" {
		return "", "", adapter.NewChannelError(adapter.ErrorKindPermanent, "slack: empty session", nil)
	}
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) == 1 {
		return parts[0], "", nil
	}
	return parts[0], parts[1], nil
}

// StartInbound implements [adapter.InboundLifecycle].
func (c *Channel) StartInbound(ctx context.Context, h adapter.InboundHandler) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case evt := <-c.client.Events:
				switch evt.Type {
				case socketmode.EventTypeInteractive:
					c.client.Ack(*evt.Request)
					cb, ok := evt.Data.(slack.InteractionCallback)
					if !ok || cb.Type != slack.InteractionTypeBlockActions {
						continue
					}
					for _, act := range cb.ActionCallback.BlockActions {
						if act == nil || !strings.HasPrefix(act.ActionID, "assistclaw_mail_") {
							continue
						}
						val := strings.TrimSpace(act.Value)
						if val == "" {
							continue
						}
						chID := cb.Channel.ID
						if chID == "" {
							continue
						}
						threadTS := strings.TrimSpace(cb.Container.ThreadTs)
						if threadTS == "" {
							threadTS = strings.TrimSpace(cb.Message.ThreadTimestamp)
						}
						sessionID := "slack:" + chID
						if threadTS != "" {
							sessionID = "slack:" + chID + ":" + threadTS
						}
						user := cb.User.ID
						if c.dmMode == "disabled" {
							continue
						}
						if c.dmMode == "allowlist" {
							allowed := false
							for _, allowedNum := range c.allowFrom {
								if user == allowedNum {
									allowed = true
									break
								}
							}
							if !allowed {
								log.Printf("Slack: blocked interactive action from unauthorized user: %s", user)
								continue
							}
						}
						inEv := adapter.InboundEvent{
							ID:         act.ActionTs,
							ChannelID:  c.Name(),
							SessionID:  sessionID,
							OccurredAt: slackTime(act.ActionTs),
							Sender: adapter.SenderRef{
								ID: user,
							},
							Recipient: adapter.RecipientRef{
								ID:   chID,
								Kind: "channel",
							},
							Text: val,
							Parts: []provider.ContentPart{{
								Type: provider.ContentTypeText,
								Text: val,
							}},
							Metadata: map[string]string{
								channels.MetaSlackChannelID: chID,
								channels.MetaSlackThreadTS:  threadTS,
							},
						}
						c.sem <- struct{}{}
						go func(inEv adapter.InboundEvent) {
							defer func() { <-c.sem }()
							if err := h(ctx, inEv); err != nil {
								log.Printf("Slack: inbound handler error: %v", err)
							}
						}(inEv)
					}
				case socketmode.EventTypeEventsAPI:
					eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok {
						continue
					}
					c.client.Ack(*evt.Request)

					switch eventsAPIEvent.Type {
					case slackevents.CallbackEvent:
						innerEvent := eventsAPIEvent.InnerEvent
						switch ev := innerEvent.Data.(type) {
						case *slackevents.MessageEvent:
							if ev.BotID != "" || ev.SubType == "bot_message" {
								break
							}

							sessionID := "slack:" + ev.Channel + ":" + ev.ThreadTimeStamp
							if ev.ThreadTimeStamp == "" {
								sessionID = "slack:" + ev.Channel
							}
							user := ev.User

							if c.dmMode == "disabled" {
								break
							}

							if c.dmMode == "allowlist" {
								allowed := false
								for _, allowedNum := range c.allowFrom {
									if user == allowedNum {
										allowed = true
										break
									}
								}
								if !allowed {
									log.Printf("Slack: blocked message from unauthorized user: %s", user)
									break
								}
							}

							occurred := slackTime(ev.TimeStamp)

							inEv := adapter.InboundEvent{
								ID:         ev.TimeStamp,
								ChannelID:  c.Name(),
								SessionID:  sessionID,
								OccurredAt: occurred,
								Sender: adapter.SenderRef{
									ID: user,
								},
								Recipient: adapter.RecipientRef{
									ID:   ev.Channel,
									Kind: "channel",
								},
								Text: ev.Text,
								Parts: []provider.ContentPart{{
									Type: provider.ContentTypeText,
									Text: ev.Text,
								}},
								Metadata: map[string]string{
									channels.MetaSlackChannelID: ev.Channel,
									channels.MetaSlackThreadTS:  ev.ThreadTimeStamp,
								},
							}

							c.sem <- struct{}{}
							go func(ev adapter.InboundEvent) {
								defer func() { <-c.sem }()
								if err := h(ctx, ev); err != nil {
									log.Printf("Slack: inbound handler error: %v", err)
								}
							}(inEv)
						}
					}
				}
			}
		}
	}()

	go func() {
		err := c.client.Run()
		if err != nil {
			log.Printf("slack run error: %v", err)
		}
	}()

	log.Printf("channels/slack: listening for incoming messages via socket mode")
	return nil
}

// Start implements [channels.Channel].
func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	return c.StartInbound(ctx, c.legacyInboundHandler(handler))
}

func (c *Channel) legacyInboundHandler(handler channels.MessageHandler) adapter.InboundHandler {
	return func(ctx context.Context, ev adapter.InboundEvent) error {
		chID := ev.Metadata[channels.MetaSlackChannelID]
		if chID == "" {
			return adapter.NewChannelError(adapter.ErrorKindPermanent, "slack: missing channel id", nil)
		}
		threadTS := ev.Metadata[channels.MetaSlackThreadTS]

		msg := channels.MessageFromInbound(ev)

		var buffer string
		replyFn := func(chunk string) error {
			buffer += chunk
			if chunk == "" || len(buffer) > 500 {
				if len(buffer) > 0 {
					if err := c.sendLegacyReplyChunk(ctx, chID, threadTS, buffer); err != nil {
						return adapter.NewChannelError(adapter.ErrorKindRetryable, "slack reply", err)
					}
					buffer = ""
				}
			}
			return nil
		}

		c.sem <- struct{}{}
		go func() {
			defer func() { <-c.sem }()
			handler(ctx, msg, replyFn, nil, nil)
		}()
		return nil
	}
}

func (c *Channel) sendLegacyReplyChunk(ctx context.Context, channelID, threadTS, text string) error {
	if c.reliableSend != nil {
		sessionID := "slack:" + channelID
		if threadTS != "" {
			sessionID += ":" + threadTS
		}
		_, err := c.reliableSend.Send(ctx, adapter.OutboundMessage{
			SessionID: sessionID,
			Text:      text,
		})
		return err
	}

	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}
	_, _, err := c.client.PostMessageContext(ctx, channelID, opts...)
	return err
}

// Stop implements [channels.Channel] and [adapter.InboundLifecycle].
func (c *Channel) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	return nil
}

func truncateMailRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n >= maxRunes {
			return strings.TrimSpace(s[:i]) + "…"
		}
		n++
	}
	return s
}

func sanitizeSlackBlockID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 200 {
			break
		}
	}
	out := b.String()
	if out == "" {
		return "actions"
	}
	return out
}

func slackBlocksFromMail(body string, kb [][]adapter.InlineKeyboardButton) slack.Blocks {
	tb := truncateMailRunes(body, 2900)
	header := slack.NewSectionBlock(slack.NewTextBlockObject(slack.PlainTextType, tb, false, false), nil, nil)
	var rows []slack.Block
	for ri, row := range kb {
		if len(row) == 0 {
			continue
		}
		var elems []slack.BlockElement
		for bi, b := range row {
			label := truncateMailRunes(strings.TrimSpace(b.Label), 75)
			if label == "" {
				label = "…"
			}
			aid := "assistclaw_mail_" + strconv.Itoa(ri) + "_" + strconv.Itoa(bi)
			val := strings.TrimSpace(b.CallbackData)
			elems = append(elems, slack.NewButtonBlockElement(aid, val, slack.NewTextBlockObject(slack.PlainTextType, label, false, false)))
		}
		if len(elems) == 0 {
			continue
		}
		bid := "assistclaw_mail_" + strconv.Itoa(ri) + "_" + sanitizeSlackBlockID(row[0].CallbackData)
		rows = append(rows, slack.NewActionBlock(bid, elems...))
	}
	if len(rows) == 0 {
		return slack.Blocks{BlockSet: []slack.Block{header}}
	}
	out := []slack.Block{header}
	out = append(out, rows...)
	return slack.Blocks{BlockSet: out}
}

// slackTime parses Slack ts strings like "1234567890.123456" to UTC.
func slackTime(ts string) time.Time {
	if ts == "" {
		return time.Now().UTC()
	}
	sec, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Now().UTC()
	}
	s := int64(sec)
	ns := int64((sec - float64(s)) * 1e9)
	return time.Unix(s, ns).UTC()
}
