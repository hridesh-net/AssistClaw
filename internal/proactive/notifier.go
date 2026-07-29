package proactive

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// WriterNotifier writes notifications to an io.Writer (e.g. stdout, log file).
// It is the simplest notifier, useful for testing and local development.
type WriterNotifier struct {
	name string
	w    io.Writer
	mu   sync.Mutex
}

// NewWriterNotifier creates a console/file notifier.
func NewWriterNotifier(name string, w io.Writer) *WriterNotifier {
	return &WriterNotifier{name: name, w: w}
}

// Name returns the notifier's identifier.
func (n *WriterNotifier) Name() string { return n.name }

// Send writes the notification body to the underlying writer.
func (n *WriterNotifier) Send(_ context.Context, notif Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	_, err := fmt.Fprintf(n.w, "[%s] rule=%s %s\n", n.name, notif.RuleID, notif.Body)
	return err
}

// ChannelNotifier wraps an AssistClaw channel adapter to send proactive notifications.
// This will be implemented in Milestone 1.2 (Telegram, Discord, etc.).
type ChannelNotifier struct {
	name   string
	sender func(ctx context.Context, sessionID, text string) error
}

// NewChannelNotifier creates a notifier backed by a channel send function.
func NewChannelNotifier(name string, sender func(ctx context.Context, sessionID, text string) error) *ChannelNotifier {
	return &ChannelNotifier{name: name, sender: sender}
}

// Name returns the notifier's identifier.
func (n *ChannelNotifier) Name() string { return n.name }

// Send delivers the notification via the channel adapter.
func (n *ChannelNotifier) Send(ctx context.Context, notif Notification) error {
	return n.sender(ctx, notif.RuleID, notif.Body)
}
