package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// robotLines is the ASCII robot, colored with ANSI via lipgloss Render.
// The bot matches the AssistClaw logo: square head, antenna, two eyes.
var robotLines = []string{
	`     ╷     `,
	`   ┌─┴─┐   `,
	`   │◉ ◉│   `,
	`   │ ▬ │   `,
	`   └──┬┘   `,
	`  ╔═══╧═╗  `,
	`  ║CLAW ║  `,
	`  ╚══╤══╝  `,
	`   ┌─┴─┐   `,
	`   └───┘   `,
}

// RenderBanner renders the full splash banner: robot + info block side by side.
func RenderBanner(ver, sessionID string, providers, skillsCount int) string {
	// Robot — lime green
	robotStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	glowStyle := lipgloss.NewStyle().Foreground(ColorNeon).Bold(true)

	var robotSB strings.Builder
	for i, line := range robotLines {
		if i == 2 || i == 3 { // eye/mouth row — neon highlight
			robotSB.WriteString(glowStyle.Render(line) + "\n")
		} else {
			robotSB.WriteString(robotStyle.Render(line) + "\n")
		}
	}

	// Info block — right pane
	subStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	infoLines := []string{
		"",
		"  " + GradientWord("ASSISTCLAW") + " " + Muted.Render(ver),
		subStyle.Render("  Edge Intelligence System"),
		"",
		dimStyle.Render("  session  ") + Primary.Render(shortID(sessionID)),
		dimStyle.Render("  providers") + Primary.Render(fmt.Sprintf("  %d", providers)),
		dimStyle.Render("  skills   ") + Primary.Render(fmt.Sprintf("  %d", skillsCount)),
		"",
		Muted.Render("  Type your message, Enter to send"),
		Muted.Render("  ESC or Ctrl+C to quit"),
	}

	var infoSB strings.Builder
	for _, l := range infoLines {
		infoSB.WriteString(l + "\n")
	}

	// Side-by-side
	combined := lipgloss.JoinHorizontal(lipgloss.Top,
		robotSB.String(),
		infoSB.String(),
	)

	// Outer border
	banner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorSurface).
		Padding(0, 1).
		Render(combined)

	// Tagline below — scanline + bracket for hacker-terminal vibe
	tagline := lipgloss.NewStyle().Foreground(ColorBorder).Render("  ") +
		CyberBracket("AUTONOMOUS EDGE CORE") + "\n  " + ScanlineSuffix(56)

	return "\n" + banner + "\n" + tagline + "\n"
}

// PrintBanner prints the splash banner to stdout.
func PrintBanner(ver, sessionID string, providers, skillsCount int) {
	fmt.Println(RenderBanner(ver, sessionID, providers, skillsCount))
}

// PrintOnboardBanner prints a shorter version for the onboarding wizard.
func PrintOnboardBanner(ver string) {
	robotStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	glowStyle := lipgloss.NewStyle().Foreground(ColorNeon).Bold(true)

	var robotSB strings.Builder
	for i, line := range robotLines {
		if i == 2 || i == 3 {
			robotSB.WriteString(glowStyle.Render(line) + "\n")
		} else {
			robotSB.WriteString(robotStyle.Render(line) + "\n")
		}
	}

	infoLines := []string{
		"",
		"  " + GradientWord("ASSISTCLAW") + " " + Muted.Render(ver),
		Primary.Render("  Setup Wizard"),
		"",
		Muted.Render("  Let's get you configured."),
		Muted.Render("  This takes about 2 minutes."),
		"",
	}
	var infoSB strings.Builder
	for _, l := range infoLines {
		infoSB.WriteString(l + "\n")
	}

	combined := lipgloss.JoinHorizontal(lipgloss.Top, robotSB.String(), infoSB.String())
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorSurface).
		Padding(0, 1).
		Render(combined)

	fmt.Println("\n" + box + "\n")
}

// shortID truncates a session ID to 8 chars for display.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
