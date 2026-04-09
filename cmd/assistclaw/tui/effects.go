package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorAccent is a secondary pop (magenta) for cyber / dev-tool contrast.
const ColorAccent = lipgloss.Color("#B388FF")

// PulseBorder returns alternating border colors for a subtle "live" frame.
func PulseBorder(frame int) lipgloss.Color {
	if frame%2 == 0 {
		return ColorPrimary
	}
	return ColorNeon
}

// GradientWord renders a short string with a horizontal green→cyan→neon sweep.
func GradientWord(s string) string {
	if s == "" {
		return ""
	}
	palette := []lipgloss.Color{ColorPrimary, ColorCyan, ColorNeon, ColorCyan, ColorPrimary}
	var b strings.Builder
	i := 0
	for _, r := range s {
		c := palette[i%len(palette)]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(true).Render(string(r)))
		i++
	}
	return b.String()
}

// CyberBracket wraps text in a terminal-hacker style frame segment.
func CyberBracket(inner string) string {
	left := lipgloss.NewStyle().Foreground(ColorAccent).Render("⟨")
	right := lipgloss.NewStyle().Foreground(ColorAccent).Render("⟩")
	mid := lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(inner)
	return left + mid + right
}

// ScanlineSuffix adds a dim CRT-style noise strip of rune width `width`.
func ScanlineSuffix(width int) string {
	if width < 8 {
		width = 32
	}
	raw := strings.Repeat("·░", (width+1)/2)
	rs := []rune(raw)
	if len(rs) > width {
		rs = rs[:width]
	}
	return lipgloss.NewStyle().Foreground(ColorBorder).Render(string(rs))
}

// RenderVersionBlock is output for `assistclaw version` (TTY).
func RenderVersionBlock(ver string) string {
	line1 := GradientWord("ASSISTCLAW") + "  " + lipgloss.NewStyle().Foreground(ColorMuted).Render(ver)
	line2 := CyberBracket("edge neural shell")
	line3 := ScanlineSuffix(48)
	return line1 + "\n" + line2 + "\n" + line3 + "\n"
}
