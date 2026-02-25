package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/mdp/qrterminal/v3"
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
				fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 40))
				fmt.Fprintln(os.Stderr, "WHATSAPP LOGIN REQUIRED")
				fmt.Fprintln(os.Stderr, strings.Repeat("=", 40))

				// Generate terminal QR code
				qrterminal.Generate(evt.Code, qrterminal.L, os.Stderr)

				fmt.Fprintln(os.Stderr, "\n1. Open WhatsApp on your phone.")
				fmt.Fprintln(os.Stderr, "2. Tap Menu or Settings and select Linked Devices.")
				fmt.Fprintln(os.Stderr, "3. Tap on Link a Device.")
				fmt.Fprintln(os.Stderr, "4. Point your phone to this screen to capture the code.")
				fmt.Fprintln(os.Stderr, strings.Repeat("=", 40)+"\n")

				// Fallback log for environments where SSH/Terminal might mangle the QR
				log.Printf("WhatsApp QR Raw Code (Backup): %s", evt.Code)
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
