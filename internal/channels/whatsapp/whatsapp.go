package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
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

func New(dbPath string, sessionID string, dmMode string, allowFrom []string, logLevel string) (*Channel, error) {
	waLevel := "WARN"
	switch strings.ToLower(logLevel) {
	case "debug":
		waLevel = "DEBUG"
	case "info":
		waLevel = "INFO"
	case "warn":
		waLevel = "WARN"
	case "error":
		waLevel = "ERROR"
	}

	dbLog := waLog.Stdout("Database", waLevel, true)
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on", dbPath)
	container, err := sqlstore.New(context.Background(), "sqlite3", dsn, dbLog)
	if err != nil {
		return nil, err
	}
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, err
	}

	clientLog := waLog.Stdout("WhatsApp", waLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	return &Channel{
		client:    client,
		sessionID: sessionID,
		dmMode:    dmMode,
		allowFrom: allowFrom,
	}, nil
}

func (c *Channel) Name() string { return "whatsapp" }

// extractText pulls the plaintext body from any WhatsApp message type.
// WhatsApp uses different fields depending on how the message was composed:
//   - Conversation:       plain text typed directly
//   - ExtendedTextMessage: text with link preview, or a reply to another message
//   - ImageMessage.Caption, VideoMessage.Caption: media with a caption
//
// We try each in order so the agent receives text regardless of message type.
func extractText(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	if t := m.GetConversation(); t != "" {
		return t
	}
	if t := m.GetExtendedTextMessage().GetText(); t != "" {
		return t
	}
	if t := m.GetImageMessage().GetCaption(); t != "" {
		return t
	}
	if t := m.GetVideoMessage().GetCaption(); t != "" {
		return t
	}
	if t := m.GetDocumentMessage().GetCaption(); t != "" {
		return t
	}
	if t := m.GetButtonsResponseMessage().GetSelectedDisplayText(); t != "" {
		return t
	}
	if t := m.GetListResponseMessage().GetTitle(); t != "" {
		return t
	}
	return ""
}

func (c *Channel) Start(ctx context.Context, handler channels.MessageHandler) error {
	c.client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {

		case *events.Message:
			// Ignore own messages
			if v.Info.IsFromMe {
				return
			}

			sender := v.Info.Sender.String()
			remoteJID := v.Info.Sender.ToNonAD().String()

			// Security policies
			if c.dmMode == "disabled" {
				return
			}
			if c.dmMode == "allowlist" {
				allowed := false
				for _, num := range c.allowFrom {
					if strings.Contains(sender, num) || strings.Contains(remoteJID, num) {
						allowed = true
						break
					}
				}
				if !allowed {
					log.Printf("WhatsApp: blocked message from %s", sender)
					return
				}
			}

			txt := extractText(v.Message)
			if txt == "" {
				// Unsupported media type — acknowledge with a note if in pairing mode
				if c.dmMode != "disabled" {
					log.Printf("WhatsApp: received non-text message from %s (type ignored)", sender)
				}
				return
			}

			// Snapshot sender JID before launching goroutine to avoid data race
			senderJID := v.Info.Sender

			// Run the agent call in a separate goroutine so we don't block
			// the WhatsApp event pump (which would cause keepalive misses
			// and eventual disconnection under load).
			go c.handleMessage(ctx, handler, senderJID, sender, txt)

		case *events.Disconnected:
			log.Printf("WhatsApp: disconnected — will auto-reconnect")

		case *events.ConnectFailure:
			log.Printf("WhatsApp: connection failure: %v", v.Reason)

		case *events.LoggedOut:
			log.Printf("WhatsApp: logged out — re-run 'assistclaw start' to re-link")
		}
	})

	return c.Connect(ctx)
}

// handleMessage runs in its own goroutine per message so the WA event loop
// is never blocked while the LLM thinks.
func (c *Channel) handleMessage(
	ctx context.Context,
	handler channels.MessageHandler,
	senderJID types.JID,
	senderStr string,
	txt string,
) {
	msg := channels.Message{
		ChannelID: c.Name(),
		SessionID: senderStr,
		Text:      txt,
	}

	replyFn := func(chunk string) error {
		if chunk == "" {
			return nil
		}
		_, err := c.client.SendMessage(ctx, senderJID, &waProto.Message{
			Conversation: proto.String(chunk),
		})
		if err != nil {
			log.Printf("WhatsApp: send error to %s: %v", senderStr, err)
		}
		return err
	}

	buf := channels.NewStreamingBuffer(replyFn, 1*time.Second)
	handler(ctx, msg, buf.Push)
	_ = buf.Done()
}

func (c *Channel) Connect(ctx context.Context) error {
	if c.client.Store.ID == nil {
		// New login — show QR and wait for pairing
		qrChan, _ := c.client.GetQRChannel(ctx)
		if err := c.client.Connect(); err != nil {
			return err
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 40))
				fmt.Fprintln(os.Stderr, "WHATSAPP LOGIN REQUIRED")
				fmt.Fprintln(os.Stderr, strings.Repeat("=", 40))
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)
				fmt.Fprintln(os.Stderr, "\n1. Open WhatsApp on your phone.")
				fmt.Fprintln(os.Stderr, "2. Tap Menu or Settings → Linked Devices.")
				fmt.Fprintln(os.Stderr, "3. Tap Link a Device.")
				fmt.Fprintln(os.Stderr, "4. Point your phone at this QR code.")
				fmt.Fprintln(os.Stderr, strings.Repeat("=", 40)+"\n")
				log.Printf("WhatsApp QR Raw Code (backup): %s", evt.Code)
			} else {
				log.Printf("WhatsApp login event: %s", evt.Event)
				if evt.Event == "success" || evt.Event == "timeout" {
					if evt.Event == "success" {
						// WA disconnects briefly after pairing for session exchange.
						// Reconnect so app-state sync runs on a live socket.
						fmt.Fprintln(os.Stderr, "\n  ✓ WhatsApp paired. Reconnecting for app-state sync...")
						c.client.Disconnect()
						time.Sleep(2 * time.Second)
						if err := c.client.Connect(); err != nil {
							return fmt.Errorf("whatsapp reconnect after pairing: %w", err)
						}
						// Wait up to 8s for connection to stabilise
						for i := 0; i < 8; i++ {
							if c.client.IsConnected() {
								break
							}
							time.Sleep(time.Second)
						}
					}
					break
				}
			}
		}
	} else {
		if err := c.client.Connect(); err != nil {
			return err
		}
	}

	fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("*", 40))
	fmt.Fprintln(os.Stderr, "WHATSAPP CONNECTED AND LISTENING")
	fmt.Fprintln(os.Stderr, strings.Repeat("*", 40)+"\n")
	log.Println("channels/whatsapp: connected and listening")
	return nil
}

func (c *Channel) IsLinked() bool { return c.client.Store.ID != nil }

func (c *Channel) Stop() error {
	c.client.Disconnect()
	return nil
}
