// Package tui provides the AssistClaw terminal UI components.
// Color palette derived from the lime-green robot logo.
package tui

import "github.com/charmbracelet/lipgloss"

// ── Palette ───────────────────────────────────────────────────────────────

const (
	ColorPrimary  = lipgloss.Color("#7CC72C") // logo lime-green
	ColorNeon     = lipgloss.Color("#AAEB47") // bright neon pop
	ColorBorder   = lipgloss.Color("#2E4020") // dim green border
	ColorSurface  = lipgloss.Color("#14180F") // dark card bg
	ColorBg       = lipgloss.Color("#0D0F0D") // near-black
	ColorMuted    = lipgloss.Color("#6B7B6B") // dimmed text
	ColorCyan     = lipgloss.Color("#44CCCC") // tool indicator
	ColorError    = lipgloss.Color("#E05A4E") // error red
	ColorWhite    = lipgloss.Color("#E8F0E8") // soft white
	ColorUserMsg  = lipgloss.Color("#AAEB47") // user bubble
	ColorAgentMsg = lipgloss.Color("#7CC72C") // agent bubble
)

// ── Base Styles ───────────────────────────────────────────────────────────

var (
	// MainBorder is the outer rounded border used in panels.
	MainBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Background(ColorSurface).
			Padding(0, 1)

	// Header is the top title bar.
	Header = lipgloss.NewStyle().
		Foreground(ColorNeon).
		Bold(true)

	// Muted is used for secondary / hint text.
	Muted = lipgloss.NewStyle().Foreground(ColorMuted)

	// Primary is used for primary highlighted text.
	Primary = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	// Neon is the brightest pop.
	Neon = lipgloss.NewStyle().Foreground(ColorNeon).Bold(true)

	// UserPrefix styles the "You ›" label.
	UserPrefix = lipgloss.NewStyle().
			Foreground(ColorUserMsg).
			Bold(true)

	// AgentPrefix styles the "🤖" label.
	AgentPrefix = lipgloss.NewStyle().
			Foreground(ColorAgentMsg).
			Bold(true)

	// ToolBadge styles the [⚙ tool_name] indicator.
	ToolBadge = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	// ErrorStyle highlights error messages.
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError).Bold(true)

	// InputLine styles the textarea border when active.
	InputBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	// StatusOK is a green dot for running status.
	StatusOK = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("●")

	// StatusErr is a red dot for stopped status.
	StatusErr = lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("●")
)

// ── Helpers ───────────────────────────────────────────────────────────────

// Badge renders a small pill label.
func Badge(text string, active bool) string {
	color := ColorMuted
	if active {
		color = ColorPrimary
	}
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(active).
		Render("  " + text)
}

// ProgressBar renders a simple ASCII progress bar.
func ProgressBar(percent float64, width int) string {
	filled := int(float64(width) * percent / 100)
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += lipgloss.NewStyle().Foreground(ColorPrimary).Render("█")
		} else {
			bar += lipgloss.NewStyle().Foreground(ColorBorder).Render("░")
		}
	}
	return bar
}

// Divider renders a full-width separator line.
func Divider(width int) string {
	line := ""
	for i := 0; i < width; i++ {
		line += "─"
	}
	return Muted.Render(line)
}
