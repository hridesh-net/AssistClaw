package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/security"
)

// auditCmd surfaces the existing HMAC-chained audit log so the user can
// see what the guardrail has actually blocked / warned / allowed without
// having to grep an opaque JSONL file by hand.
func auditCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect the security audit log",
		Long: `Show entries from the HMAC-chained audit log (default: most recent 20).

Every tool call, skill access, and guardrail decision is recorded with a
tamper-evident hash chain. Use this command to spot-check what the agent
has been doing or to confirm that guardrail blocks are firing as expected.`,
	}
	cmd.AddCommand(auditListCmd(gf), auditTailCmd(gf))
	return cmd
}

func auditListCmd(gf *globalFlags) *cobra.Command {
	var (
		limit       int
		eventFilter string
		jsonOut     bool
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "Show the most recent audit entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveAuditPath(gf.configPath)
			if err != nil {
				return err
			}
			entries, err := readLastEntries(path, limit, eventFilter)
			if err != nil {
				return err
			}
			return printEntries(entries, jsonOut)
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 20, "Number of entries to show")
	c.Flags().StringVar(&eventFilter, "event", "", "Filter by event type (e.g. guardrail_block, tool_call)")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print raw NDJSON entries")
	return c
}

func auditTailCmd(gf *globalFlags) *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use:   "tail",
		Short: "Follow the audit log in real time",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveAuditPath(gf.configPath)
			if err != nil {
				return err
			}
			return tailEntries(path, follow)
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", true, "Keep reading as new entries arrive")
	return c
}

func resolveAuditPath(cfgPath string) (string, error) {
	cfg, err := loadConfig(cfgPath, buildLogger("warn"))
	if err != nil {
		return "", err
	}
	if cfg.Security.LogPath != "" {
		return cfg.Security.LogPath, nil
	}
	return filepath.Join(cfg.StateDir, "security", "audit.ndjson"), nil
}

// readLastEntries returns up to `limit` entries from the end of the log.
// `eventFilter` (if non-empty) keeps only entries whose Event matches.
// We stream the file line-by-line to avoid loading multi-MB logs into RAM.
func readLastEntries(path string, limit int, eventFilter string) ([]security.AuditEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no audit log at %s — run the agent at least once", path)
		}
		return nil, err
	}
	defer f.Close()

	// Ring buffer of `limit` entries so we don't keep the whole log.
	if limit <= 0 {
		limit = 20
	}
	ring := make([]security.AuditEntry, 0, limit)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // up to 4 MiB per line
	for scanner.Scan() {
		var e security.AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if eventFilter != "" && string(e.Event) != eventFilter {
			continue
		}
		if len(ring) < limit {
			ring = append(ring, e)
		} else {
			copy(ring, ring[1:])
			ring[limit-1] = e
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ring, nil
}

func printEntries(entries []security.AuditEntry, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	}
	dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
	prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
	red := func(s string) string { return tui.Style(tui.ColorError, true, s) }
	cyan := func(s string) string { return tui.Style(tui.ColorCyan, true, s) }

	if len(entries) == 0 {
		fmt.Println(dim("  (no matching entries)"))
		return nil
	}
	for _, e := range entries {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
		seq := dim(fmt.Sprintf("#%-6d", e.Seq))
		event := formatEvent(string(e.Event), prim, red, cyan)
		line := fmt.Sprintf("%s  %s  %s", dim(ts), seq, event)
		if e.Tool != "" {
			line += "  tool=" + prim(e.Tool)
		}
		if e.Skill != "" {
			line += "  skill=" + cyan(e.Skill)
		}
		if e.Node != "" {
			line += dim("  node=" + e.Node)
		}
		if e.Guardrail != nil {
			line += dim(fmt.Sprintf("  action=%s findings=[%s]",
				e.Guardrail.Action,
				strings.Join(e.Guardrail.Findings, ",")))
		}
		if e.DurationMs > 0 {
			line += dim("  " + strconv.FormatInt(e.DurationMs, 10) + "ms")
		}
		fmt.Println(line)
	}
	return nil
}

func formatEvent(event string, prim, red, cyan func(string) string) string {
	switch event {
	case string(security.EventGuardrailBlock):
		return red("BLOCK         ")
	case string(security.EventGuardrailWarn):
		return red("WARN          ")
	case string(security.EventToolCall):
		return prim("tool_call     ")
	case string(security.EventSkillRead):
		return cyan("skill_read    ")
	case string(security.EventSkillIndex):
		return cyan("skill_index   ")
	case string(security.EventSessionStart):
		return prim("session_start ")
	case string(security.EventSessionEnd):
		return prim("session_end   ")
	default:
		return fmt.Sprintf("%-14s", event)
	}
}

// tailEntries prints existing entries then (if follow) blocks reading new
// lines appended to the file. Polling-based — good enough for an audit
// stream that rarely exceeds a few writes per second.
func tailEntries(path string, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no audit log at %s — run the agent at least once", path)
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
	for scanner.Scan() {
		emitOne(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !follow {
		return nil
	}

	fmt.Println(dim("--- following new entries (Ctrl+C to stop) ---"))
	pos, _ := f.Seek(0, 1) // current offset
	for {
		time.Sleep(500 * time.Millisecond)
		stat, err := f.Stat()
		if err != nil {
			return err
		}
		if stat.Size() <= pos {
			continue
		}
		if _, err := f.Seek(pos, 0); err != nil {
			return err
		}
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for s.Scan() {
			emitOne(s.Bytes())
		}
		if err := s.Err(); err != nil {
			return err
		}
		pos, _ = f.Seek(0, 1)
	}
}

func emitOne(line []byte) {
	var e security.AuditEntry
	if err := json.Unmarshal(line, &e); err != nil {
		return
	}
	_ = printEntries([]security.AuditEntry{e}, false)
}

// Suppress "unused" warning for config in non-default callers.
var _ = config.DefaultConfigPath
