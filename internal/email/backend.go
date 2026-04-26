package email

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/assistclaw/assistclaw/internal/config"
)

// Ref identifies one message in a mailbox.
type Ref struct {
	AccountName string
	ProviderID  string // IMAP UID, Gmail message id, Graph message id
}

// MailMessage is a normalized fetched mail for rules + LLM.
type MailMessage struct {
	Ref         Ref
	From        string
	Subject     string
	BodyText    string
	MessageID   string // RFC Message-ID for threading
	InReplyTo   string
	References  string
	GmailLabels []string // optional, for rule matching
	GraphCats   []string // optional
}

// Backend watches a mailbox and can fetch messages and send replies.
// There is intentionally no Delete method — mail deletion is not supported.
type Backend interface {
	Name() string
	// Watch blocks until ctx is cancelled, calling onNew when new mail arrives.
	Watch(ctx context.Context, onNew func(context.Context, Ref) error) error
	Fetch(ctx context.Context, ref Ref) (*MailMessage, error)
	Reply(ctx context.Context, m *MailMessage, body string) error
}

type BackendBuilder func(cfg *config.Config, acc config.EmailAccountConfig, store *Store) (Backend, error)

var (
	backendBuildersMu sync.RWMutex
	backendBuilders   = map[string]BackendBuilder{}
)

// RegisterBackendBuilder registers a backend constructor by name (e.g. imap, gmail, graph).
// Extensions can call this during init to add new backends without changing core dispatch.
func RegisterBackendBuilder(name string, b BackendBuilder) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || b == nil {
		return
	}
	backendBuildersMu.Lock()
	backendBuilders[key] = b
	backendBuildersMu.Unlock()
}

// NewBackendForAccount builds a backend from YAML account config.
func NewBackendForAccount(cfg *config.Config, acc config.EmailAccountConfig, store *Store) (Backend, error) {
	key := strings.ToLower(strings.TrimSpace(acc.Backend))
	backendBuildersMu.RLock()
	b, ok := backendBuilders[key]
	backendBuildersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown email backend %q", acc.Backend)
	}
	return b(cfg, acc, store)
}
