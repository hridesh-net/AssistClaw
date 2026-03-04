// Package security — tamper-evident HMAC hash chain audit log.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EventType classifies what happened in an audit event.
type EventType string

const (
	EventToolCall       EventType = "tool_call"
	EventSkillRead      EventType = "skill_read"
	EventSkillIndex     EventType = "skill_index"
	EventGuardrailWarn  EventType = "guardrail_warn"
	EventGuardrailBlock EventType = "guardrail_block"
	EventSessionStart   EventType = "session_start"
	EventSessionEnd     EventType = "session_end"
)

// AuditEntry is a single record in the audit log.
type AuditEntry struct {
	Seq        uint64            `json:"seq"`
	Timestamp  time.Time         `json:"ts"`
	SessionID  string            `json:"session"`
	ChannelID  string            `json:"channel"`
	Event      EventType         `json:"event"`
	Actor      string            `json:"actor,omitempty"` // username/id from channel, if known
	Tool       string            `json:"tool,omitempty"`
	Skill      string            `json:"skill,omitempty"`
	Node       string            `json:"node,omitempty"`
	InputHash  string            `json:"input_hash,omitempty"`  // sha256 of input (not plain text)
	OutputHash string            `json:"output_hash,omitempty"` // sha256 of output
	DurationMs int64             `json:"duration_ms,omitempty"`
	Guardrail  *GuardrailSummary `json:"guardrail,omitempty"`
	PrevHash   string            `json:"prev_hash"`  // hash of previous entry
	EntryHash  string            `json:"entry_hash"` // HMAC(secret, prev_hash+content)
}

// GuardrailSummary is a compact form of the guardrail result for the audit log.
type GuardrailSummary struct {
	Findings []string `json:"findings,omitempty"` // rule names only
	Action   Action   `json:"action"`
}

// AuditLog writes tamper-evident audit entries to an NDJSON file.
type AuditLog struct {
	mu       sync.Mutex
	f        *os.File
	path     string
	secret   []byte // HMAC key derived from machine ID
	seq      uint64
	prevHash string
	log      *zap.Logger
	piiMask  bool
}

// NewAuditLog opens (or creates) the audit log at the given path.
func NewAuditLog(path string, piiMask bool, log *zap.Logger) (*AuditLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("audit: create dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit: open log: %w", err)
	}

	secret, err := loadOrCreateSecret(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("audit: secret: %w", err)
	}

	al := &AuditLog{
		f:        f,
		path:     path,
		secret:   secret,
		prevHash: "genesis",
		log:      log,
		piiMask:  piiMask,
	}

	// Scan to the last entry's hash + seq so we can continue the chain.
	al.seq, al.prevHash = scanLastEntry(path, secret)

	return al, nil
}

// Close flushes and closes the log file.
func (al *AuditLog) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.f.Close()
}

// WriteToolCall logs a tool execution event.
func (al *AuditLog) WriteToolCall(sessionID, channelID, actor, toolName string,
	inputJSON []byte, output string, dur time.Duration, result CheckResult) {
	al.write(AuditEntry{
		SessionID:  sessionID,
		ChannelID:  channelID,
		Actor:      actor,
		Event:      EventToolCall,
		Tool:       toolName,
		InputHash:  hashString(string(inputJSON)),
		OutputHash: hashString(output),
		DurationMs: dur.Milliseconds(),
		Guardrail:  summariseResult(result),
	})
}

// WriteSkillRead logs a read_skill_node event.
func (al *AuditLog) WriteSkillRead(sessionID, channelID, actor, skill, node string, dur time.Duration) {
	al.write(AuditEntry{
		SessionID:  sessionID,
		ChannelID:  channelID,
		Actor:      actor,
		Event:      EventSkillRead,
		Skill:      skill,
		Node:       node,
		DurationMs: dur.Milliseconds(),
	})
}

// WriteSkillIndex logs a skill_graph_index event.
func (al *AuditLog) WriteSkillIndex(sessionID, channelID, actor string, skills []string) {
	al.write(AuditEntry{
		SessionID: sessionID,
		ChannelID: channelID,
		Actor:     actor,
		Event:     EventSkillIndex,
		Skill:     fmt.Sprintf("[%s]", joinStrings(skills, ",")),
	})
}

// WriteGuardrailEvent logs a guardrail finding event.
func (al *AuditLog) WriteGuardrailEvent(sessionID, channelID string, result CheckResult) {
	evt := EventGuardrailWarn
	if result.Action == ActionBlock {
		evt = EventGuardrailBlock
	}
	al.write(AuditEntry{
		SessionID: sessionID,
		ChannelID: channelID,
		Event:     evt,
		Guardrail: summariseResult(result),
	})
}

// WriteSessionStart logs a session start event.
func (al *AuditLog) WriteSessionStart(sessionID, channelID string) {
	al.write(AuditEntry{
		SessionID: sessionID,
		ChannelID: channelID,
		Event:     EventSessionStart,
	})
}

// WriteSessionEnd logs a session end event.
func (al *AuditLog) WriteSessionEnd(sessionID, channelID string) {
	al.write(AuditEntry{
		SessionID: sessionID,
		ChannelID: channelID,
		Event:     EventSessionEnd,
	})
}

// write serialises an entry, computes the HMAC, and appends it.
func (al *AuditLog) write(e AuditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	al.seq++
	e.Seq = al.seq
	e.Timestamp = time.Now().UTC()
	e.PrevHash = al.prevHash

	// Compute content hash before setting EntryHash
	content := entryContent(e)
	e.EntryHash = computeHMAC(al.secret, al.prevHash+content)

	line, err := json.Marshal(e)
	if err != nil {
		al.log.Error("audit: marshal error", zap.Error(err))
		return
	}
	if _, err := al.f.Write(append(line, '\n')); err != nil {
		al.log.Error("audit: write error", zap.Error(err))
		return
	}

	al.prevHash = e.EntryHash
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

func computeHMAC(secret []byte, data string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return "hmac:" + hex.EncodeToString(mac.Sum(nil))
}

// entryContent builds the stable string used as HMAC input —
// excludes EntryHash itself to avoid circularity.
func entryContent(e AuditEntry) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		e.Seq, e.Timestamp.Format(time.RFC3339Nano),
		e.SessionID, e.ChannelID, e.Event,
		e.Tool, e.Skill, e.Node,
		e.InputHash, e.OutputHash,
	)
}

func summariseResult(r CheckResult) *GuardrailSummary {
	if len(r.Findings) == 0 && r.Action == ActionAllow {
		return nil
	}
	s := &GuardrailSummary{Action: r.Action}
	for _, f := range r.Findings {
		s.Findings = append(s.Findings, f.Rule)
	}
	return s
}

// loadOrCreateSecret loads the HMAC secret from dir/audit.secret, creating it
// if missing. Secret is 32 random bytes, stored hex-encoded.
func loadOrCreateSecret(dir string) ([]byte, error) {
	path := filepath.Join(dir, "audit.secret")
	data, err := os.ReadFile(path)
	if err == nil {
		raw, e := hex.DecodeString(string(data))
		if e == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	// Generate new secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	hexStr := hex.EncodeToString(secret)
	if err := os.WriteFile(path, []byte(hexStr), 0600); err != nil {
		return nil, err
	}
	return secret, nil
}

// scanLastEntry reads the audit log to find the last valid seq + hash.
func scanLastEntry(path string, secret []byte) (uint64, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "genesis"
	}
	lines := splitLines(string(data))
	var lastSeq uint64
	lastHash := "genesis"
	for _, line := range lines {
		line = trimLine(line)
		if line == "" {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Seq > lastSeq {
			lastSeq = e.Seq
			lastHash = e.EntryHash
		}
	}
	return lastSeq, lastHash
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimLine(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
