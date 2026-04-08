package adapter

import (
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// ThreadRef identifies a thread or reply chain in a channel-specific way.
// Semantics differ by provider (Slack thread_ts vs Teams reply chain, etc.).
type ThreadRef struct {
	// ID is the opaque provider thread identifier.
	ID string
	// ParentMessageID optionally identifies the root/parent message in the thread.
	ParentMessageID string
}

// Attachment describes an inbound file or media handle without necessarily loading bytes.
type Attachment struct {
	ID       string
	Kind     string // e.g. "image", "file", "audio"
	URL      string
	MimeType string
	SizeBytes int64
}

// SenderRef identifies who sent an inbound event.
type SenderRef struct {
	ID          string // opaque channel user id
	DisplayName string
	Username    string
}

// RecipientRef identifies the conversation target (DM, channel, space).
type RecipientRef struct {
	ID   string
	Kind string // e.g. "dm", "channel", "group", "space"
}

// InboundEvent is a normalized inbound message or signal from a channel.
// It is the v1 payload for [InboundHandler]; legacy [channels.Message] can be mapped to this shape during migration.
type InboundEvent struct {
	ID        string
	ChannelID string // same as [Identity.Name] / adapter name, e.g. "telegram"
	SessionID string // AssistClaw session key (e.g. tg:123)

	OccurredAt time.Time

	Sender    SenderRef
	Recipient RecipientRef

	Text string
	// Parts mirrors multimodal content when present; Text may duplicate plain text for convenience.
	Parts []provider.ContentPart

	ThreadRef *ThreadRef
	Attachments []Attachment
	// Metadata holds small string-keyed channel-specific fields (ids, reply references).
	Metadata map[string]string
}

// ChannelCapabilities describes what an integration supports. Used by runners and UI (see STORY-003 registry).
type ChannelCapabilities struct {
	Threading        bool
	Attachments      bool
	DirectMessages   bool
	GroupMessages    bool
	Reactions        bool
	Edits            bool
	MaxMessageLength int // 0 means unknown / no fixed limit
}

// OutboundMessage is a request to send content to a channel. IdempotencyKey is used by reliability layers (STORY-002).
type OutboundMessage struct {
	IdempotencyKey string
	SessionID      string
	Text           string
	Parts          []provider.ContentPart
	ThreadRef      *ThreadRef
	ReplyToID      string // optional provider message id to reply to
}

// DeliveryReceipt confirms an outbound send. ProviderMessageID is the remote id when available.
type DeliveryReceipt struct {
	ProviderMessageID string
	IdempotencyKey    string
	SentAt            time.Time
}
