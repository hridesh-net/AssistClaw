// Package tui provides the AssistClaw terminal UI bindings.
//
// Banner / version / interactive screens are implemented in Rust
// (cmd/assistclaw/tui_rs) and reached through the C FFI in tui_rs.go.
// The Go side keeps a small pure-ANSI helper (this file) for inline
// coloured output by other CLI commands so they can drop direct
// lipgloss usage.
package tui

import (
	"fmt"
	"strings"
)

// ── Palette (hex RGB, matches Rust tui_rs/src/render.rs) ──────────────────

const (
	HexPrimary = "#7CC72C" // logo lime-green
	HexNeon    = "#AAEB47" // bright neon pop
	HexBorder  = "#2E4020" // dim green border
	HexSurface = "#14180F" // dark card bg
	HexMuted   = "#6B7B6B" // dimmed text
	HexCyan    = "#44CCCC" // tool indicator
	HexError   = "#E05A4E" // error red
	HexWhite   = "#E8F0E8" // soft white
	HexAccent  = "#B388FF" // accent magenta
)

// Backwards-compat aliases that callers across the cmd tree depend on.
// Kept as plain strings (hex codes) so callers can pass them to Style().
const (
	ColorPrimary = HexPrimary
	ColorNeon    = HexNeon
	ColorBorder  = HexBorder
	ColorSurface = HexSurface
	ColorMuted   = HexMuted
	ColorCyan    = HexCyan
	ColorError   = HexError
	ColorWhite   = HexWhite

	// Logical aliases retained for legacy call sites.
	ColorUserMsg  = HexNeon
	ColorAgentMsg = HexPrimary
)

const ansiReset = "\x1b[0m"

func parseHex(hex string) (r, g, b int) {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		return 0xE8, 0xF0, 0xE8
	}
	_, _ = fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	return
}

// Style applies a truecolor foreground (optionally bold) to text and resets.
func Style(hex string, bold bool, text string) string {
	r, g, b := parseHex(hex)
	if bold {
		return fmt.Sprintf("\x1b[1;38;2;%d;%d;%dm%s%s", r, g, b, text, ansiReset)
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s%s", r, g, b, text, ansiReset)
}

// Style256 applies an ANSI 256-color foreground (optionally bold).
// `code` is the 0–255 palette index as a numeric string ("42", "212", …),
// matching the legacy lipgloss.Color("N") shorthand.
func Style256(code string, bold bool, text string) string {
	if bold {
		return fmt.Sprintf("\x1b[1;38;5;%sm%s%s", code, text, ansiReset)
	}
	return fmt.Sprintf("\x1b[38;5;%sm%s%s", code, text, ansiReset)
}

// Bold wraps text in ANSI bold without changing colour.
func Bold(text string) string { return "\x1b[1m" + text + ansiReset }

// Faint wraps text in ANSI faint without changing colour.
func Faint(text string) string { return "\x1b[2m" + text + ansiReset }

// Render* helpers mirror the most-used lipgloss styles.
func RenderPrimary(text string) string { return Style(HexPrimary, true, text) }
func RenderNeon(text string) string    { return Style(HexNeon, true, text) }
func RenderMuted(text string) string   { return Style(HexMuted, false, text) }
func RenderError(text string) string   { return Style(HexError, true, text) }
func RenderHeader(text string) string  { return Style(HexNeon, true, text) }

// Stylish exposes the lipgloss-like `.Render(text)` shape so legacy call sites
// that used `tui.Muted.Render(s)` keep compiling without a sweep.
type Stylish struct {
	Hex  string
	Bold bool
}

func (s Stylish) Render(text string) string { return Style(s.Hex, s.Bold, text) }

var (
	Muted       = Stylish{Hex: HexMuted}
	Primary     = Stylish{Hex: HexPrimary, Bold: true}
	Neon        = Stylish{Hex: HexNeon, Bold: true}
	Header      = Stylish{Hex: HexNeon, Bold: true}
	UserPrefix  = Stylish{Hex: HexNeon, Bold: true}
	AgentPrefix = Stylish{Hex: HexPrimary, Bold: true}
	ToolBadge   = Stylish{Hex: HexCyan, Bold: true}
	ErrorStyle  = Stylish{Hex: HexError, Bold: true}
)

// StatusOK / StatusErr are dot indicators used by status dashboards.
var (
	StatusOK  = Style(HexPrimary, true, "●")
	StatusErr = Style(HexError, true, "●")
)

// Badge renders a small pill label, optionally highlighted.
func Badge(text string, active bool) string {
	if active {
		return Style(HexPrimary, true, "  "+text)
	}
	return Style(HexMuted, false, "  "+text)
}

// Divider renders a full-width separator line.
func Divider(width int) string {
	return RenderMuted(strings.Repeat("─", width))
}

// ProgressBar renders a simple ASCII progress bar with the brand palette.
func ProgressBar(percent float64, width int) string {
	filled := int(float64(width) * percent / 100)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString(Style(HexPrimary, false, "█"))
		} else {
			b.WriteString(Style(HexBorder, false, "░"))
		}
	}
	return b.String()
}
