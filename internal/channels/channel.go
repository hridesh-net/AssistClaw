package channels

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// SlashCommandLine extracts the first line that looks like a slash command (/word …).
// Use this for chat command routing: bridges may prepend "[CHAT INFO]…" or WhatsApp may
// include quoted reply text above the user's "/new" line — msg.Text alone may not start with "/".
func SlashCommandLine(msg Message) string {
	raw := strings.TrimSpace(msg.Text)
	raw = strings.TrimPrefix(raw, "\ufeff")
	if raw == "" {
		for _, p := range msg.Parts {
			if p.Type == provider.ContentTypeText {
				t := strings.TrimSpace(p.Text)
				if t != "" {
					raw = t
					break
				}
			}
		}
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	for len(raw) > 0 {
		r, w := utf8.DecodeRuneInString(raw)
		if r == '\ufeff' || r == '\u200f' || r == '\u200e' { // BOM, RLM, LRM
			raw = strings.TrimSpace(raw[w:])
			continue
		}
		break
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(line, "/") {
			return line
		}
	}
	return ""
}

// Message represents an inbound communication from a channel.
type Message struct {
	ID        string // Unique message ID from the channel
	ChannelID string // e.g., "telegram", "discord"
	SessionID string // Unique identifier for the conversation/user
	Text      string // Message payload
	Parts     []provider.ContentPart
	Metadata  map[string]any
}

// StreamingReplyFunc is a callback provided to the message handler,
// allowing it to send chunks of text (tokens) back to the channel in real-time.
type StreamingReplyFunc func(chunk string) error

// ReactionFunc allows sending an emoji reaction to a specific message.
type ReactionFunc func(emoji string) error

// MediaReplyFunc allows sending media (images, files) back to the user.
type MediaReplyFunc func(data []byte, fileName string, mimeType string) error

// MessageHandler is the callback for incoming messages.
type MessageHandler func(ctx context.Context, msg Message, reply StreamingReplyFunc, react ReactionFunc, media MediaReplyFunc)

// Context keys
type contextKey string

const (
	MediaFnKey contextKey = "channels.MediaFn"
)

// Channel defines the interface for a messaging platform integration.
type Channel interface {
	Name() string

	// Start connects to the channel and begins listening for messages,
	// dispatching them to the provided handler.
	Start(ctx context.Context, handler MessageHandler) error

	// Stop gracefully disconnects from the channel.
	Stop() error
}
