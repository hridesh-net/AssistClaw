package proactive

import (
	"context"
	"time"

	"github.com/assistclaw/assistclaw/internal/email"
	"go.uber.org/zap"
)

// EmailTrigger uses the existing email backend Watch to emit events when new mail arrives.
type EmailTrigger struct {
	account string
	backend email.Backend
	log     *zap.Logger
}

// NewEmailTrigger creates an email trigger backed by an existing email backend.
func NewEmailTrigger(account string, backend email.Backend, log *zap.Logger) *EmailTrigger {
	if log == nil {
		log = zap.NewNop()
	}
	return &EmailTrigger{account: account, backend: backend, log: log}
}

// Name returns the trigger identifier.
func (e *EmailTrigger) Name() string { return "email:" + e.account }

// Start blocks on the backend's Watch, emitting an event for each new message.
func (e *EmailTrigger) Start(ctx context.Context, emit EmitFunc) error {
	return e.backend.Watch(ctx, func(watchCtx context.Context, ref email.Ref) error {
		msg, err := e.backend.Fetch(watchCtx, ref)
		if err != nil {
			e.log.Warn("email trigger fetch failed", zap.String("account", e.account), zap.Error(err))
			return nil // Don't stop the watcher on fetch errors.
		}

		importance := "normal"
		if email.ActionFor(nil, msg) == email.ActionNotifyOnly {
			importance = "important"
		}

		emit(Event{
			Source: e.Name(),
			Type:   "received",
			Payload: map[string]any{
				"from":       msg.From,
				"subject":    msg.Subject,
				"body":       msg.BodyText,
				"importance": importance,
				"provider_id": ref.ProviderID,
			},
			Time: time.Now(),
		})
		return nil
	})
}
