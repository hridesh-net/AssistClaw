package channels

import "context"

// Message represents an inbound communication from a channel.
type Message struct {
	ChannelID string // e.g., "telegram", "discord"
	SessionID string // Unique identifier for the conversation/user
	Text      string // Message payload
}

// StreamingReplyFunc is a callback provided to the message handler,
// allowing it to send chunks of text (tokens) back to the channel in real-time.
type StreamingReplyFunc func(chunk string) error

// MessageHandler processes an inbound message.
type MessageHandler func(ctx context.Context, msg Message, reply StreamingReplyFunc)

// Channel defines the interface for a messaging platform integration.
type Channel interface {
	Name() string

	// Start connects to the channel and begins listening for messages,
	// dispatching them to the provided handler.
	Start(ctx context.Context, handler MessageHandler) error

	// Stop gracefully disconnects from the channel.
	Stop() error
}
