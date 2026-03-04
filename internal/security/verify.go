package security

import (
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// VerifyResult is the output of a full audit log verification.
type VerifyResult struct {
	TotalEntries   int
	ValidEntries   int
	Violations     []VerifyViolation
	FirstTimestamp time.Time
	LastTimestamp  time.Time
}

// VerifyViolation describes a single failing entry.
type VerifyViolation struct {
	Seq    uint64
	LineNo int
	Reason string
}

func (v VerifyResult) OK() bool { return len(v.Violations) == 0 }

// VerifyLog reads the audit log at path and verifies the HMAC hash chain.
// Returns a VerifyResult describing all valid and violated entries.
func VerifyLog(path string) (VerifyResult, error) {
	secretPath := strings.TrimSuffix(path, "audit.ndjson") + "audit.secret"

	// Allow the secret to be in the same directory regardless of filename
	dir := dirOf(path)
	secretPath = dir + "/audit.secret"

	hexSecret, err := os.ReadFile(secretPath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("cannot read audit secret at %s: %w", secretPath, err)
	}
	secret, err := hexDecode(string(hexSecret))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("invalid audit secret format: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("cannot read audit log: %w", err)
	}

	lines := splitLines(string(data))
	result := VerifyResult{}
	prevHash := "genesis"
	expectedSeq := uint64(0)

	for lineNo, raw := range lines {
		line := trimLine(raw)
		if line == "" {
			continue
		}
		result.TotalEntries++

		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			result.Violations = append(result.Violations, VerifyViolation{
				LineNo: lineNo + 1,
				Reason: "JSON parse error: " + err.Error(),
			})
			continue
		}

		expectedSeq++

		// Verify sequence
		if e.Seq != expectedSeq {
			result.Violations = append(result.Violations, VerifyViolation{
				Seq:    e.Seq,
				LineNo: lineNo + 1,
				Reason: fmt.Sprintf("sequence gap: expected %d, got %d", expectedSeq, e.Seq),
			})
			expectedSeq = e.Seq // re-sync to continue checking
		}

		// Verify prev_hash chain
		if e.PrevHash != prevHash {
			result.Violations = append(result.Violations, VerifyViolation{
				Seq:    e.Seq,
				LineNo: lineNo + 1,
				Reason: fmt.Sprintf("prev_hash mismatch at seq %d — entry may have been inserted or deleted", e.Seq),
			})
		}

		// Verify HMAC
		content := entryContent(e)
		expectedMAC := computeHMAC(secret, prevHash+content)
		if !hmacEqual(e.EntryHash, expectedMAC) {
			result.Violations = append(result.Violations, VerifyViolation{
				Seq:    e.Seq,
				LineNo: lineNo + 1,
				Reason: fmt.Sprintf("HMAC mismatch at seq %d — entry was tampered with", e.Seq),
			})
		} else {
			result.ValidEntries++
		}

		// Track timestamps
		if result.FirstTimestamp.IsZero() {
			result.FirstTimestamp = e.Timestamp
		}
		result.LastTimestamp = e.Timestamp

		prevHash = e.EntryHash
	}

	return result, nil
}

// FormatVerifyResult returns a human-readable verification report.
func FormatVerifyResult(path string, r VerifyResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Audit log: %s\n", path))
	sb.WriteString(fmt.Sprintf("Entries:   %d total, %d verified\n", r.TotalEntries, r.ValidEntries))
	if !r.FirstTimestamp.IsZero() {
		sb.WriteString(fmt.Sprintf("Range:     %s → %s\n",
			r.FirstTimestamp.Format("2006-01-02 15:04:05 MST"),
			r.LastTimestamp.Format("2006-01-02 15:04:05 MST"),
		))
	}

	if r.OK() {
		sb.WriteString("\n✓ Chain integrity verified — no tampering detected.\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n✗ %d violation(s) detected:\n", len(r.Violations)))
		for _, v := range r.Violations {
			sb.WriteString(fmt.Sprintf("  • Line %d (seq %d): %s\n", v.LineNo, v.Seq, v.Reason))
		}
	}
	return sb.String()
}

// SummaryReport returns a high-level summary of audit events by type.
func SummaryReport(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read audit log: %w", err)
	}

	counts := map[EventType]int{}
	tools := map[string]int{}
	skills := map[string]int{}
	actors := map[string]int{}
	warnings := 0
	blocks := 0
	total := 0

	for _, raw := range splitLines(string(data)) {
		line := trimLine(raw)
		if line == "" {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		total++
		counts[e.Event]++
		if e.Tool != "" {
			tools[e.Tool]++
		}
		if e.Skill != "" && e.Node != "" {
			skills[e.Skill+"/"+e.Node]++
		}
		if e.Actor != "" {
			actors[e.Actor]++
		}
		if e.Event == EventGuardrailWarn {
			warnings++
		}
		if e.Event == EventGuardrailBlock {
			blocks++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Security Audit Report ===\nTotal events: %d\n\n", total))
	sb.WriteString("Events by type:\n")
	for t, n := range counts {
		sb.WriteString(fmt.Sprintf("  %-25s %d\n", t, n))
	}
	if len(tools) > 0 {
		sb.WriteString("\nTop tools:\n")
		for t, n := range tools {
			sb.WriteString(fmt.Sprintf("  %-25s %d calls\n", t, n))
		}
	}
	if len(skills) > 0 {
		sb.WriteString("\nSkill node reads:\n")
		for s, n := range skills {
			sb.WriteString(fmt.Sprintf("  %-35s %d reads\n", s, n))
		}
	}
	if len(actors) > 0 {
		sb.WriteString("\nActors:\n")
		for a, n := range actors {
			sb.WriteString(fmt.Sprintf("  %-25s %d events\n", a, n))
		}
	}
	sb.WriteString(fmt.Sprintf("\nGuardrail: %d warnings, %d blocks\n", warnings, blocks))
	return sb.String(), nil
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func hexDecode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex string")
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, lo := hexVal(s[i]), hexVal(s[i+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex char at pos %d", i)
		}
		out[i/2] = byte(hi<<4 | lo)
	}
	return out, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// hmacEqual compares two HMAC hex strings in constant time.
func hmacEqual(a, b string) bool {
	// Strip "hmac:" prefix before comparing
	aRaw := strings.TrimPrefix(a, "hmac:")
	bRaw := strings.TrimPrefix(b, "hmac:")
	aBytes, err1 := hexDecode(aRaw)
	bBytes, err2 := hexDecode(bRaw)
	if err1 != nil || err2 != nil {
		return false
	}
	return hmac.Equal(aBytes, bBytes)
}
