package whatsapp

import (
	"context"
	"fmt"
	"log"

	"github.com/assistclaw/assistclaw/internal/channels"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Channel struct {
	client    *whatsmeow.Client
	sessionID string
}

func New(dbPath string, sessionID string) (*Channel, error) {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Context, Dialect, DSN, Logger
	container, err := sqlstore.New(context.Background(), "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		return nil, err
	}
	// Context
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, err
	}

	client := whatsmeow.NewClient(deviceStore, dbLog)
	return &Channel{
		client:    client,
		sessionID: sessionID,
	}, nil
}

func (c *Channel) Name() string {
	return "whatsapp"
}

func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	c.client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if v.Info.IsFromMe {
				return
			}

			txt := v.Message.GetConversation()
			if txt == "" {
				return
			}

			msg := channels.Message{
				ChannelID: c.Name(),
				SessionID: v.Info.Sender.String(),
				Text:      txt,
			}

			replyFn := func(chunk string) error {
				if chunk == "" {
					return nil
				}
				_, err := c.client.SendMessage(ctx, v.Info.Sender, &waProto.Message{
					Conversation: proto.String(chunk),
				})
				return err
			}

			handler(ctx, msg, replyFn)
		}
	})

	if c.client.Store.ID == nil {
		// New login needed
		qrChan, _ := c.client.GetQRChannel(ctx)
		err := c.client.Connect()
		if err != nil {
			return err
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n--- WHATSAPP LOGIN REQUIRED ---")
				fmt.Printf("WhatsApp QR Code: %s\n", evt.Code)
				fmt.Println("To login, visit https://web.whatsapp.com and scan this code,")
				fmt.Println("or use a QR code generator for: " + evt.Code)
				fmt.Println("--------------------------------\n")
			} else {
				log.Printf("WhatsApp login event: %s", evt.Event)
			}
		}
	} else {
		err := c.client.Connect()
		if err != nil {
			return err
		}
	}

	log.Println("channels/whatsapp: connected and listening")
	return nil
}

func (c *Channel) Stop() error {
	c.client.Disconnect()
	return nil
}
