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

// GoalStatus is the lifecycle state of an email goal.
type GoalStatus string

const (
	// GoalActive: the goal is open and we are composing or assessing.
	GoalActive GoalStatus = "active"
	// GoalWaitingReply: our mail was sent; waiting on the counterpart.
	GoalWaitingReply GoalStatus = "waiting_reply"
	// GoalAchieved: the objective was met; thread is closed.
	GoalAchieved GoalStatus = "achieved"
	// GoalBlocked: the counterpart refused or progress is impossible without the user.
	GoalBlocked GoalStatus = "blocked"
	// GoalAbandoned: cancelled by the user.
	GoalAbandoned GoalStatus = "abandoned"
)

// IsOpen reports whether the goal still participates in inbound routing and follow-ups.
func (st GoalStatus) IsOpen() bool {
	return st == GoalActive || st == GoalWaitingReply || st == GoalBlocked
}

// Goal is a persistent objective AssistClaw pursues over an email thread:
// it drafts the opening mail, processes replies, follows up on silence, and
// reports when the objective is achieved. Every outbound send still requires
// explicit user approval via the standard draft tokens.
type Goal struct {
	ID            int64
	AccountName   string
	Counterpart   string // email address of the other party
	Subject       string
	Objective     string
	Status        GoalStatus
	FollowupAfter time.Duration // nudge the counterpart after this much silence
	MaxFollowups  int
	FollowupsSent int
	ThreadRefs    []string // Message-IDs seen or sent on this thread
	LastActivity  time.Time
	CreatedAt     time.Time
}

// GoalEvent is one audit/transcript entry for a goal.
type GoalEvent struct {
	ID        int64
	GoalID    int64
	Kind      string // created | inbound | draft_created | sent | achieved | blocked | abandoned | note
	Detail    string
	CreatedAt time.Time
}
