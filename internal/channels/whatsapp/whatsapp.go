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
	dmMode    string
	allowFrom []string
}

func New(dbPath string, sessionID string, dmMode string, allowFrom []string) (*Channel, error) {
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
		dmMode:    dmMode,
		allowFrom: allowFrom,
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

			sender := v.Info.Sender.String()
			remoteJID := v.Info.Sender.ToNonAD().String()

			// Enforce security policies
			if c.dmMode == "disabled" {
				return
			}

			if c.dmMode == "allowlist" {
				allowed := false
				for _, allowedNum := range c.allowFrom {
					// Check if sender contains the allowed number (e.g. 1234567890 in 1234567890@s.whatsapp.net)
					if strings.Contains(sender, allowedNum) || strings.Contains(remoteJID, allowedNum) {
						allowed = true
						break
					}
				}
				if !allowed {
					log.Printf("WhatsApp: blocked message from unauthorized sender: %s", sender)
					return
				}
			}

			txt := v.Message.GetConversation()
			if txt == "" {
				return
			}

			msg := channels.Message{
				ChannelID: c.Name(),
				SessionID: sender,
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

	return c.Connect(ctx)
}

func (c *Channel) Connect(ctx context.Context) error {
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

				// Generate smaller terminal QR code using half-blocks
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)

				fmt.Fprintln(os.Stderr, "\n1. Open WhatsApp on your phone.")
				fmt.Fprintln(os.Stderr, "2. Tap Menu or Settings and select Linked Devices.")
				fmt.Fprintln(os.Stderr, "3. Tap on Link a Device.")
				fmt.Fprintln(os.Stderr, "4. Point your phone to this screen to capture the code.")
				fmt.Fprintln(os.Stderr, strings.Repeat("=", 40)+"\n")

				// Fallback log for environments where SSH/Terminal might mangle the QR
				log.Printf("WhatsApp QR Raw Code (Backup): %s", evt.Code)
			} else {
				log.Printf("WhatsApp login event: %s", evt.Event)
				if evt.Event == "success" || evt.Event == "timeout" {
					break
				}
			}
		}
	} else {
		err := c.client.Connect()
		if err != nil {
			return err
		}
	}

	fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("*", 40))
	fmt.Fprintln(os.Stderr, "WHATSAPP CONNECTED AND LISTENING")
	fmt.Fprintln(os.Stderr, strings.Repeat("*", 40)+"\n")
	log.Println("channels/whatsapp: connected and listening")
	return nil
}

func (c *Channel) Stop() error {
	c.client.Disconnect()
	return nil
}
