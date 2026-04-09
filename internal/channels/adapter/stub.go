package adapter

import (
	"context"
	"sync"
	"time"
)

// Stub is a minimal [Adapter] implementation for tests and scaffolding. It does not connect to any network.
type Stub struct {
	ChannelName string
	Caps        ChannelCapabilities

	mu     sync.Mutex
	stopCh chan struct{}
}

// Compile-time check: Stub implements Adapter.
var _ Adapter = (*Stub)(nil)

// Name returns ChannelName or "stub" if empty.
func (s *Stub) Name() string {
	if s.ChannelName != "" {
		return s.ChannelName
	}
	return "stub"
}

// AdapterVersion implements Identity.
func (s *Stub) AdapterVersion() int { return Version1 }

// Capabilities implements Identity.
func (s *Stub) Capabilities() ChannelCapabilities {
	if s.Caps.Threading || s.Caps.Attachments || s.Caps.MaxMessageLength != 0 {
		return s.Caps
	}
	return ChannelCapabilities{
		Threading:      true,
		Attachments:    true,
		DirectMessages: true,
		GroupMessages:  true,
		Mentions:       true,
		Voice:          false,
	}
}

// StartInbound implements InboundLifecycle (no inbound traffic).
func (s *Stub) StartInbound(ctx context.Context, _ InboundHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopCh != nil {
		return NewChannelError(ErrorKindPermanent, "stub already started", nil)
	}
	s.stopCh = make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-s.stopCh:
		}
	}()
	return nil
}

// Stop implements InboundLifecycle.
func (s *Stub) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
		s.stopCh = nil
	}
	return nil
}

// Send implements OutboundSender with a synthetic receipt (for tests).
func (s *Stub) Send(ctx context.Context, msg OutboundMessage) (*DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewChannelError(ErrorKindRetryable, "context done", err)
	}
	key := msg.IdempotencyKey
	if key == "" {
		key = "none"
	}
	return &DeliveryReceipt{
		ProviderMessageID: "stub-msg",
		IdempotencyKey:    key,
		SentAt:            time.Now().UTC(),
	}, nil
}

// Ping implements Health.
func (s *Stub) Ping(ctx context.Context) error {
	return ctx.Err()
}
