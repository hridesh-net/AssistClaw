// Package awareness gives AssistClaw a live model of the user's current
// situation — time of day, upcoming calendar events, presence at the machine,
// and arbitrary external signals (phone, sensors). The store renders a compact
// context block that the agent runner injects into every system prompt, so the
// assistant answers knowing *now*, not just the conversation.
package awareness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Signal is one live fact about the user's situation.
type Signal struct {
	Value     string        `json:"value"`
	UpdatedAt time.Time     `json:"updated_at"`
	TTL       time.Duration `json:"ttl"` // 0 = never expires
}

// Expired reports whether the signal has outlived its TTL.
func (s Signal) Expired(now time.Time) bool {
	return s.TTL > 0 && now.Sub(s.UpdatedAt) > s.TTL
}

// Store is a thread-safe signal store with JSON snapshot persistence, so
// awareness survives daemon restarts.
type Store struct {
	mu      sync.RWMutex
	signals map[string]Signal
	path    string // snapshot file; empty = memory only
}

// NewStore creates a store persisting to stateDir/awareness.json and loads
// any previous snapshot. Pass an empty stateDir for a memory-only store.
func NewStore(stateDir string) *Store {
	s := &Store{signals: map[string]Signal{}}
	if stateDir != "" {
		s.path = filepath.Join(stateDir, "awareness.json")
		s.load()
	}
	return s
}

// Set records a signal. TTL 0 means the signal never expires.
func (s *Store) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	s.signals[key] = Signal{Value: value, UpdatedAt: time.Now(), TTL: ttl}
	s.persistLocked()
	s.mu.Unlock()
}

// Delete removes a signal.
func (s *Store) Delete(key string) {
	s.mu.Lock()
	delete(s.signals, key)
	s.persistLocked()
	s.mu.Unlock()
}

// Get returns a live (non-expired) signal.
func (s *Store) Get(key string) (Signal, bool) {
	s.mu.RLock()
	sig, ok := s.signals[key]
	s.mu.RUnlock()
	if !ok || sig.Expired(time.Now()) {
		return Signal{}, false
	}
	return sig, true
}

// Snapshot returns all live signals keyed by name.
func (s *Store) Snapshot() map[string]Signal {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Signal, len(s.signals))
	for k, sig := range s.signals {
		if !sig.Expired(now) {
			out[k] = sig
		}
	}
	return out
}

// Keys returns live signal names, sorted for deterministic rendering.
func (s *Store) Keys() []string {
	snap := s.Snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// persistLocked writes the snapshot file. Caller holds s.mu.
func (s *Store) persistLocked() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s.signals, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var sigs map[string]Signal
	if err := json.Unmarshal(data, &sigs); err != nil {
		return
	}
	s.mu.Lock()
	s.signals = sigs
	s.mu.Unlock()
}
