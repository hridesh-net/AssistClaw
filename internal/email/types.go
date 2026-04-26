package email

import "time"

// DraftStatus is persisted in the email store.
type DraftStatus string

const (
	DraftPending  DraftStatus = "pending"
	DraftSent     DraftStatus = "sent"
	DraftRejected DraftStatus = "rejected"
)

// StoredMessage is a row in email_messages.
type StoredMessage struct {
	ID           int64
	AccountName  string
	ProviderMsgID string
	FromAddr     string
	Subject      string
	Snippet      string
	CreatedAt    time.Time
}

// StoredDraft is a row in email_drafts.
type StoredDraft struct {
	ID        int64
	MessageID int64
	Token     string
	Summary   string
	Body      string
	Status    DraftStatus
	CreatedAt time.Time
}
