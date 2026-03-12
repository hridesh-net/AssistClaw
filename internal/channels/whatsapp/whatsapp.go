package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/voice"
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
	voice     *voice.Client
}

func New(dbPath string, sessionID string, dmMode string, allowFrom []string, logLevel string, voiceClient *voice.Client) (*Channel, error) {
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
		voice:     voiceClient,
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

// extractMultimodal extracts both text and media parts from a message.
func (c *Channel) extractMultimodal(ctx context.Context, m *waProto.Message) ([]provider.ContentPart, string) {
	if m == nil {
		return nil, ""
	}

	var parts []provider.ContentPart
	txt := extractText(m)

	// Always add the text part if present
	if txt != "" {
		parts = append(parts, provider.ContentPart{
			Type: provider.ContentTypeText,
			Text: txt,
		})
	}

	// Handle Image Media
	if img := m.GetImageMessage(); img != nil {
		data, err := c.client.Download(ctx, img)
		if err == nil {
			parts = append(parts, provider.ContentPart{
				Type:          provider.ContentTypeImage,
				ImageData:     data,
				ImageMimeType: img.GetMimetype(),
			})
		} else {
			log.Printf("WhatsApp: failed to download image: %v", err)
		}
	}

	// Handle Audio Media (Voice Notes)
	if aud := m.GetAudioMessage(); aud != nil {
		data, err := c.client.Download(ctx, aud)
		if err == nil {
			parts = append(parts, provider.ContentPart{
				Type:          provider.ContentTypeAudio,
				ImageData:     data, // Store in ImageData or a new field if we want to be explicit
				ImageMimeType: aud.GetMimetype(),
			})

			// pro-actively transcribe if voice client is available
			if c.voice != nil {
				// try to determine format from mimetype
				format := "ogg"
				if strings.Contains(aud.GetMimetype(), "mp4") {
					format = "mp4"
				}
				transcription, err := c.voice.STT(data, format)
				if err == nil && transcription != "" {
					txt += "\n[Voice Note]: " + transcription
					parts = append(parts, provider.ContentPart{
						Type: provider.ContentTypeText,
						Text: "[Voice Note Transcription]: " + transcription,
					})
				} else {
					log.Printf("WhatsApp: transcription failed or empty: %v", err)
				}
			}

			// If we have an audio part but no text, add a fallback to avoid agent errors
			if txt == "" {
				txt = "[Audio Message]"
			}
		} else {
			log.Printf("WhatsApp: failed to download audio: %v", err)
		}
	}

	return parts, txt
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

			parts, txt := c.extractMultimodal(ctx, v.Message)
			if len(parts) == 0 {
				// Unsupported media type — acknowledge with a note if in pairing mode
				if c.dmMode != "disabled" {
					log.Printf("WhatsApp: received non-text message from %s (type ignored)", sender)
				}
				return
			}

			// Snapshot sender and chat JIDs before launching goroutine
			senderJID := v.Info.Sender
			chatJID := v.Info.Chat
			msgID := v.Info.ID

			// Run the agent call in a separate goroutine so we don't block
			// the WhatsApp event pump (which would cause keepalive misses
			// and eventual disconnection under load).
			go c.handleMessage(ctx, senderJID, chatJID, msgID, sender, txt, parts, handler)

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
	senderJID types.JID,
	chatJID types.JID,
	msgID string,
	senderStr string,
	txt string,
	parts []provider.ContentPart,
	msgHandler channels.MessageHandler,
) {
	sessionID := senderStr
	if chatJID.Server == "g.us" {
		sessionID = chatJID.String() // Shared context for groups
	}

	msg := channels.Message{
		ID:        msgID,
		ChannelID: c.Name(),
		SessionID: sessionID,
		Text:      txt,
		Parts:     parts,
	}

	replyFn := func(chunk string) error {
		if chunk == "" {
			return nil
		}

		// If the incoming message was audio, try to reply with audio (continuous conversation)
		isAudio := false
		for _, p := range parts {
			if p.Type == provider.ContentTypeAudio {
				isAudio = true
				break
			}
		}

		if isAudio && c.voice != nil {
			voiceData, err := c.voice.TTS(chunk, "")
			if err == nil {
				resp, err := c.client.Upload(ctx, voiceData, whatsmeow.MediaAudio)
				if err == nil {
					_, err = c.client.SendMessage(ctx, chatJID, &waProto.Message{
						AudioMessage: &waProto.AudioMessage{
							URL:           proto.String(resp.URL),
							DirectPath:    proto.String(resp.DirectPath),
							MediaKey:      resp.MediaKey,
							Mimetype:      proto.String("audio/ogg; codecs=opus"),
							FileSHA256:    resp.FileSHA256,
							FileEncSHA256: resp.FileEncSHA256,
							FileLength:    proto.Uint64(resp.FileLength),
							PTT:           proto.Bool(true), // Send as voice note
						},
					})
					return err
				}
			}
			log.Printf("WhatsApp: voice synthesis/upload failed: %v", err)
		}

		// Fallback to text if not audio or audio failed
		const maxLen = 4000
		var err error
		for len(chunk) > 0 {
			cut := len(chunk)
			if cut > maxLen {
				cut = maxLen
			}
			part := chunk[:cut]
			chunk = chunk[cut:]

			_, sendErr := c.client.SendMessage(ctx, chatJID, &waProto.Message{
				Conversation: proto.String(part),
			})
			if sendErr != nil {
				log.Printf("WhatsApp: send error to %s: %v", senderStr, sendErr)
				err = sendErr
			}
		}
		return err
	}

	reactFn := func(emoji string) error {
		_, err := c.client.SendMessage(ctx, chatJID, &waProto.Message{
			ReactionMessage: &waProto.ReactionMessage{
				Key: &waProto.MessageKey{
					RemoteJID: proto.String(chatJID.String()),
					FromMe:    proto.Bool(false),
					ID:        proto.String(msgID),
					Participant: func() *string {
						if chatJID.Server == "g.us" {
							return proto.String(senderJID.String())
						}
						return nil
					}(),
				},
				Text:              proto.String(emoji),
				GroupingKey:       proto.String(emoji),
				SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
			},
		})
		return err
	}

	buf := channels.NewStreamingBuffer(replyFn, 500*time.Millisecond)
	mediaFn := func(data []byte, fileName string, mimeType string) error {
		// Upload media to WhatsApp servers
		resp, err := c.client.Upload(ctx, data, whatsmeow.MediaImage) // Default to Image for now
		if err != nil {
			return fmt.Errorf("whatsapp: failed to upload media: %w", err)
		}

		// Send message with media
		msg := &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileSHA256:    resp.FileSHA256,
				FileEncSHA256: resp.FileEncSHA256,
				FileLength:    proto.Uint64(resp.FileLength),
			},
		}
		_, err = c.client.SendMessage(ctx, chatJID, msg)
		return err
	}

	msgHandler(ctx, msg, buf.Push, reactFn, mediaFn)
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
						fmt.Fprintln(os.Stderr, "\n  ✓ WhatsApp successfully linked. Waiting 10s for initial app-state sync...")
						// Wait before returning control so onboard.go doesn't close
						// the socket while the phone is negotiating device keys.
						time.Sleep(10 * time.Second)
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
