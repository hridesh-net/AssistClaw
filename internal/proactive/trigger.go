package proactive

import (
	"context"
	"sync"
)

// ManualTrigger is a test seam and admin hook that lets callers inject events
// directly into the engine without a long-running producer.
type ManualTrigger struct {
	mu     sync.Mutex
	emit   EmitFunc
	closed bool
}

// NewManualTrigger creates a trigger that can be fired on demand.
func NewManualTrigger() *ManualTrigger {
	return &ManualTrigger{}
}

// Name returns "manual".
func (m *ManualTrigger) Name() string { return "manual" }

// Start captures the emit callback so Fire can enqueue events.
func (m *ManualTrigger) Start(_ context.Context, emit EmitFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emit = emit
	m.closed = false
	return nil
}

// Fire injects an event into the engine. Safe to call from any goroutine.
func (m *ManualTrigger) Fire(ev Event) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.emit == nil {
		return false
	}
	m.emit(ev)
	return true
}

// Close marks the trigger as shut down. Further Fire calls are dropped.
func (m *ManualTrigger) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.emit = nil
}
