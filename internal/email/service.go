package email

import (
	"context"
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/tools"
	"go.uber.org/zap"
)

// Service runs email watchers and approval dispatch.
type Service struct {
	cfg       *config.Config
	emailCfg  config.EmailConfig
	store     *Store
	p         provider.Provider
	modelID   string
	notify    config.EmailNotifyConfig
	accounts  []config.EmailAccountConfig
	senders   map[string]tools.ChannelSender
	reliable  map[string]*adapter.ReliableSender // lower channel name -> wrapper (optional; enables inline approvals)
	backends  map[string]Backend                // account name -> backend
	log       *zap.Logger
}

// NewService wires the email subsystem. Call Run from the daemon.
// When cfg.Email.Enabled is false, returns (nil, nil).
func NewService(
	cfg *config.Config,
	store *Store,
	p provider.Provider,
	modelID string,
	senders map[string]tools.ChannelSender,
	reliable map[string]*adapter.ReliableSender,
	log *zap.Logger,
) (*Service, error) {
	if cfg == nil || !cfg.Email.Enabled {
		return nil, nil
	}
	if reliable == nil {
		reliable = map[string]*adapter.ReliableSender{}
	}
	backs := make(map[string]Backend)
	for _, acc := range cfg.Email.Accounts {
		be, err := NewBackendForAccount(cfg, acc, store)
		if err != nil {
			return nil, fmt.Errorf("email account %q: %w", acc.Name, err)
		}
		backs[acc.Name] = be
	}
	return &Service{
		cfg:      cfg,
		emailCfg: cfg.Email,
		store:    store,
		p:        p,
		modelID:  modelID,
		notify:   cfg.Email.Notify,
		accounts: cfg.Email.Accounts,
		senders:  senders,
		reliable: reliable,
		backends: backs,
		log:      log,
	}, nil
}

func (s *Service) accountByName(name string) (config.EmailAccountConfig, bool) {
	for _, a := range s.accounts {
		if a.Name == name {
			return a, true
		}
	}
	return config.EmailAccountConfig{}, false
}

func (s *Service) publishText(ctx context.Context, text string) error {
	ch := strings.ToLower(strings.TrimSpace(s.notify.Channel))
	sender, ok := s.senders[ch]
	if !ok || sender == nil {
		return fmt.Errorf("no channel sender for %q", s.notify.Channel)
	}
	return sender.SendText(ctx, s.notify.SessionID, text)
}

// inboundMatchesNotify returns true if this DM/thread is configured for mail approvals.
func (s *Service) inboundMatchesNotify(channelID, sessionID string) bool {
	if s == nil {
		return false
	}
	if strings.EqualFold(channelID, s.notify.Channel) && sessionID == s.notify.SessionID {
		return true
	}
	for _, a := range s.accounts {
		if a.Notify != nil && strings.EqualFold(channelID, a.Notify.Channel) && sessionID == a.Notify.SessionID {
			return true
		}
	}
	return false
}

// WrapHandler returns a MessageHandler that handles mail approvals first.
func (s *Service) WrapHandler(next channels.MessageHandler) channels.MessageHandler {
	if s == nil {
		return next
	}
	return func(ctx context.Context, msg channels.Message, reply channels.StreamingReplyFunc, react channels.ReactionFunc, media channels.MediaReplyFunc) {
		if s.inboundMatchesNotify(msg.ChannelID, msg.SessionID) {
			replyText, handled, err := s.HandleInboundCommand(ctx, msg.ChannelID, msg.SessionID, msg.Text)
			if err != nil {
				_ = reply("Email handler error: " + err.Error())
				return
			}
			if handled {
				if replyText != "" {
					_ = reply(replyText)
				}
				return
			}
		}
		next(ctx, msg, reply, react, media)
	}
}

// Run starts one goroutine per account watching mail.
func (s *Service) Run(ctx context.Context) {
	for _, acc := range s.accounts {
		acc := acc
		be := s.backends[acc.Name]
		if be == nil {
			continue
		}
		go func() {
			err := be.Watch(ctx, func(c context.Context, ref Ref) error {
				ref.AccountName = acc.Name
				m, err := be.Fetch(c, ref)
				if err != nil {
					s.log.Warn("email fetch", zap.String("account", acc.Name), zap.Error(err))
					return err
				}
				if err := s.ProcessNewMail(c, acc, m); err != nil {
					s.log.Warn("email pipeline", zap.String("account", acc.Name), zap.Error(err))
					return err
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				s.log.Warn("email watch ended", zap.String("account", acc.Name), zap.Error(err))
			}
		}()
	}
}
