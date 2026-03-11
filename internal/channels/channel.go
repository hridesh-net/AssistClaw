package channels

import (
	"context"

	"github.com/assistclaw/assistclaw/internal/provider"
)

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
