package slack

import (
	"context"
	"fmt"
	"log"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/assistclaw/assistclaw/internal/channels"
)

// Channel implements channels.Channel for Slack using SocketMode
type Channel struct {
	client *socketmode.Client
	stopCh chan struct{}
}

func New(botToken, appToken string) (*Channel, error) {
	if botToken == "" || appToken == "" {
		return nil, fmt.Errorf("slack bot token and app token are required")
	}

	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	client := socketmode.New(api)

	return &Channel{
		client: client,
		stopCh: make(chan struct{}),
	}, nil
}

func (c *Channel) Name() string {
	return "slack"
}

func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case evt := <-c.client.Events:
				switch evt.Type {
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
							// Ignore messages from bots
							if ev.BotID != "" || ev.SubType == "bot_message" {
								break
							}

							sessionID := fmt.Sprintf("slack:%s:%s", ev.Channel, ev.ThreadTimeStamp)
							if ev.ThreadTimeStamp == "" {
								sessionID = fmt.Sprintf("slack:%s", ev.Channel)
							}

							msg := channels.Message{
								ChannelID: c.Name(),
								SessionID: sessionID,
								Text:      ev.Text,
							}

							var buffer string
							replyFn := func(chunk string) error {
								buffer += chunk
								if chunk == "" || len(buffer) > 500 {
									if len(buffer) > 0 {
										opts := []slack.MsgOption{slack.MsgOptionText(buffer, false)}
										if ev.ThreadTimeStamp != "" {
											opts = append(opts, slack.MsgOptionTS(ev.ThreadTimeStamp))
										}
										c.client.PostMessage(ev.Channel, opts...)
										buffer = ""
									}
								}
								return nil
							}

							go func() {
								handler(ctx, msg, replyFn)
								if buffer != "" {
									opts := []slack.MsgOption{slack.MsgOptionText(buffer, false)}
									if ev.ThreadTimeStamp != "" {
										opts = append(opts, slack.MsgOptionTS(ev.ThreadTimeStamp))
									}
									c.client.PostMessage(ev.Channel, opts...)
								}
							}()
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

func (c *Channel) Stop() error {
	close(c.stopCh)
	// slack client doesn't have a direct Close for socketmode, context cancellation typically handles it
	return nil
}
