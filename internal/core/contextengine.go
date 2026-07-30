package core

import "context"

// Tokens is a token count. Named for clarity at call sites that pass budgets
// and pressures around.
type Tokens int

// InjectionPosition says where an ephemeral injection is placed relative to the
// stable, cache-friendly prompt prefix.
type InjectionPosition int

const (
	// InjectSystemSuffix places the injection as the last system block — after
	// the stable prefix (system + identity + skills + tool table), so the prefix
	// stays byte-identical across turns and remains prompt-cacheable.
	InjectSystemSuffix InjectionPosition = iota
	// InjectUserPrefix places the injection at the front of the next user turn.
	InjectUserPrefix
)

// Injection is a piece of volatile context (a retrieved memory, an awareness
// signal, a nudge) added to a turn without mutating the cacheable prefix.
type Injection struct {
	Source   string            // where it came from ("memory", "awareness", "nudge")
	Content  string            // the text to inject
	Position InjectionPosition // where it goes relative to the stable prefix
	Score    float64           // relevance score, for ordering/trimming under budget
}

// TurnContext is the read-only view a ContextEngine needs to decide what to
// inject and how hard to compress for the turn about to run.
type TurnContext struct {
	UserID      string    // stable user identity — memory spans surfaces, not sessions
	SessionID   string    // the current session/surface
	Surface     string    // "chat" | "cowork" | "code" | ...
	Messages    []Message // working-memory messages for this turn
	ModelCtxMax Tokens    // the active model's context window
	Query       string    // the current user message text
}

// ContextEngine manages what goes into the model's context window each turn:
// which volatile context to inject, and when/how to shrink history under
// pressure. It is deliberately model-light (prune/compact before summarizing)
// so it is cheap on edge devices. This is the Hermes pluggable-context-engine
// pattern; the default implementation lives in internal/contextengine (WS3).
type ContextEngine interface {
	// ShouldCompress reports whether accumulated pressure warrants shrinking
	// history before the next request. Pressure is measured pre-request from the
	// request being built — never from lagging provider-reported token counts.
	ShouldCompress(pressure, budget Tokens) bool

	// Compress returns a shorter message list. Implementations should prune old
	// tool results first, then compact the middle, and only summarize via an LLM
	// as a last resort — and never mid tool-call chain.
	Compress(ctx context.Context, msgs []Message) ([]Message, error)

	// SelectContext returns the volatile injections for this turn (retrieved
	// memories, awareness signals, nudges), ordered by Score. It must not mutate
	// the cacheable prefix.
	SelectContext(ctx context.Context, turn TurnContext) []Injection

	// PruneToolResults stubs out tool results older than the last keepLast turns,
	// leaving a placeholder the model can act on. The cheapest first line of
	// defense against context blowouts.
	PruneToolResults(msgs []Message, keepLast int) []Message
}
