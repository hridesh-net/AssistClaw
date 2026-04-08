// Package adapter defines the versioned AssistClaw channel integration contract (Adapter v1).
//
// Legacy integrations implement [github.com/assistclaw/assistclaw/internal/channels.Channel]
// with [github.com/assistclaw/assistclaw/internal/channels.MessageHandler]; new work should target
// Adapter v1 and map into the legacy path until migration completes (see doc/architecture/channel-adapter-v1.md).
//
// Errors: use [ChannelError] and [ErrorKind]; do not rely on error strings for classification.
package adapter

import "context"

const (
	// Version1 is the current Adapter contract version.
	Version1 = 1
)

// Adapter is the full v1 contract: identity, inbound loop, outbound send, and health checks.
// Prefer this type when registering integrations; use smaller interfaces only when splitting implementations.
type Adapter interface {
	Identity
	InboundLifecycle
	OutboundSender
	Health
}

// Identity names the integration for config, metrics, and logs.
type Identity interface {
	// Name returns a stable identifier (e.g. "telegram", "slack", "msteams").
	Name() string
	// AdapterVersion returns the contract version; use [Version1] for this package's types.
	AdapterVersion() int
	// Capabilities declares supported features for routing and UI.
	Capabilities() ChannelCapabilities
}

// InboundLifecycle receives normalized inbound events via [InboundHandler] until [Stop] or context cancel.
// The method is named StartInbound (not Start) so the same concrete type can also implement
// [github.com/assistclaw/assistclaw/internal/channels.Channel], which uses Start(ctx, MessageHandler).
type InboundLifecycle interface {
	// StartInbound begins listening. Implementations must respect ctx cancellation.
	StartInbound(ctx context.Context, handler InboundHandler) error
	// Stop releases resources. It must be safe to call more than once.
	Stop() error
}

// InboundHandler receives one normalized [InboundEvent] per inbound message (or channel-defined unit of work).
type InboundHandler func(ctx context.Context, ev InboundEvent) error

// OutboundSender sends assistant output to the channel. Runners may wrap this with retries (STORY-002).
type OutboundSender interface {
	// Send delivers an outbound message. Errors must use [ChannelError] classification where possible.
	Send(ctx context.Context, msg OutboundMessage) (*DeliveryReceipt, error)
}

// Health supports preflight and operator checks (doctor, probes).
type Health interface {
	// Ping verifies credentials / connectivity with a short, read-only operation when possible.
	Ping(ctx context.Context) error
}
