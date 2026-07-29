package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ─────────────────────────────────────────────
// Public config passed in from statusCmd
// ─────────────────────────────────────────────

// StatusInfo holds the static config that doesn't change between refreshes.
type StatusInfo struct {
	PID           int
	Version       string
	SkillSummary  string
	Channels      []string
	PlanoEnabled  bool
	PlanoEndpoint string
	MCPEnabled    bool
	MCPTransport  string
	// NoMouse disables mouse capture in the dashboard. Set when the user
	// is inside tmux/screen or passed --no-mouse.
	NoMouse bool
}

// ─────────────────────────────────────────────
// Process stats fetching
// ─────────────────────────────────────────────

// procStats is the cross-platform process snapshot consumed by the status
// dashboard. An `err` value means the dashboard should display an error
// banner rather than a misleading "0.0% CPU" row.
type procStats struct {
	cpuPct float64
	rssMB  float64
	etime  string
	alive  bool
	err    string
}

// fetchStats returns a live snapshot of the running daemon. On Linux it
// prefers /proc/<pid>/stat + /proc/<pid>/status (machine-readable, no
// platform-dependent column parsing). On every other OS it falls back to
// `ps -p PID -o %cpu,rss,etime` with strict parsing.
func fetchStats(pid int) procStats {
	if pid <= 0 {
		return procStats{err: "no daemon pid"}
	}
	if runtime.GOOS == "linux" {
		if s, ok := fetchStatsLinux(pid); ok {
			return s
		}
	}
	return fetchStatsPS(pid)
}

// fetchStatsLinux reads /proc/<pid>/{stat,statm,uptime} — fields are
// stable across Linux kernels and don't depend on the busybox/coreutils
// flavour of `ps`.
func fetchStatsLinux(pid int) (procStats, bool) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	stat, err := os.ReadFile(statPath)
	if err != nil {
		return procStats{err: "process not running"}, true
	}

	// `stat` format puts comm in parentheses; everything after the closing
	// ')' is space-separated and stable. Split off the prefix safely so a
	// command name containing spaces or ')' does not corrupt parsing.
	close := strings.LastIndexByte(string(stat), ')')
	if close < 0 || close+2 > len(stat) {
		return procStats{err: "unreadable /proc stat"}, true
	}
	fields := strings.Fields(string(stat)[close+2:])
	// After the trimmed prefix, fields are 0-indexed as documented in
	// proc(5). We need utime (11), stime (12), starttime (19), rss (21).
	if len(fields) < 22 {
		return procStats{err: "unexpected /proc stat shape"}, true
	}
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)
	starttime, _ := strconv.ParseInt(fields[19], 10, 64)
	rssPages, _ := strconv.ParseInt(fields[21], 10, 64)

	uptime, err := readUptimeSeconds()
	if err != nil {
		return procStats{err: "cannot read /proc/uptime"}, true
	}
	clkTck := float64(clockTicksPerSec())
	procStart := float64(starttime) / clkTck
	procSec := uptime - procStart
	if procSec <= 0 {
		procSec = 1
	}
	cpuPct := 100.0 * (float64(utime+stime) / clkTck) / procSec

	pageSize := os.Getpagesize() // bytes per page; Linux normally 4096
	rssBytes := rssPages * int64(pageSize)
	rssMB := float64(rssBytes) / (1024.0 * 1024.0)

	return procStats{
		cpuPct: cpuPct,
		rssMB:  rssMB,
		etime:  formatDuration(procSec),
		alive:  true,
	}, true
}

func readUptimeSeconds() (float64, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty /proc/uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// clockTicksPerSec is the kernel jiffy rate. On glibc this is reachable via
// sysconf(_SC_CLK_TCK), but for portability across libc we just hard-code
// the value that every mainstream Linux kernel uses (100 Hz). If the
// user has rebuilt the kernel with HZ=250, the CPU% will be off by a
// constant factor — acceptable for a dashboard.
func clockTicksPerSec() int64 { return 100 }

// fetchStatsPS is the macOS / BSD / fallback path. It calls `ps` with an
// explicit column list and validates the result; any parse failure
// surfaces as a populated `err` string instead of silently-zero stats.
func fetchStatsPS(pid int) procStats {
	cmd := exec.Command("ps", "-p", fmt.Sprint(pid), "-o", "%cpu=,rss=,etime=")
	out, err := cmd.Output()
	if err != nil {
		return procStats{err: "process not running"}
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return procStats{err: "process not running"}
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return procStats{err: "unparseable ps output"}
	}
	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return procStats{err: "ps cpu field not numeric"}
	}
	rssKB, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return procStats{err: "ps rss field not numeric"}
	}
	return procStats{
		cpuPct: cpu,
		rssMB:  float64(rssKB) / 1024.0,
		etime:  fields[2],
		alive:  true,
	}
}

func formatDuration(secs float64) string {
	d := time.Duration(secs * float64(time.Second))
	h := int(d / time.Hour)
	m := int(d/time.Minute) % 60
	s := int(d/time.Second) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func tickEvery() *time.Ticker { return time.NewTicker(time.Second) }

// ─────────────────────────────────────────────
// Entry point
// ─────────────────────────────────────────────

// RunStatus launches the live-updating status dashboard via the Rust TUI.
// Exits when the user presses q, Esc, or Ctrl+C.
func RunStatus(info StatusInfo) error {
	infoMap := map[string]any{
		"pid":            info.PID,
		"version":        info.Version,
		"skill_summary":  info.SkillSummary,
		"channels":       info.Channels,
		"plano_enabled":  info.PlanoEnabled,
		"plano_endpoint": info.PlanoEndpoint,
		"mcp_enabled":    info.MCPEnabled,
		"mcp_transport":  info.MCPTransport,
	}
	if info.NoMouse {
		infoMap["enable_mouse"] = false
	}
	infoJSON, _ := json.Marshal(infoMap)

	if err := Init(); err != nil {
		return fmt.Errorf("init tui: %w", err)
	}
	defer Shutdown()

	done := make(chan error, 1)
	go func() { done <- StatusRun(string(infoJSON)) }()

	ticker := tickEvery()
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			stats := fetchStats(info.PID)
			ramPct := (stats.rssMB / 1024.0) * 100.0
			if ramPct > 100 {
				ramPct = 100
			}
			payload := map[string]any{
				"cpu_pct": stats.cpuPct,
				"ram_mb":  stats.rssMB,
				"ram_pct": ramPct,
				"alive":   stats.alive,
				"etime":   stats.etime,
			}
			if stats.err != "" {
				payload["error"] = stats.err
			}
			statusJSON, _ := json.Marshal(payload)
			StatusUpdate(string(statusJSON))
		}
	}()

	return <-done
}
