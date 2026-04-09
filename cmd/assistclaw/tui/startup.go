package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ArgsWantNoColor returns true if the process args include a no-color flag (parsed before Cobra runs).
func ArgsWantNoColor() bool {
	for _, a := range os.Args[1:] {
		switch a {
		case "--no-color", "--no-colour":
			return true
		}
	}
	return false
}

// ShouldPrintStartupBanner is false for version/help invocations so stderr stays quiet.
func ShouldPrintStartupBanner() bool {
	args := os.Args[1:]
	for _, a := range args {
		switch a {
		case "-h", "--help", "--version", "-v":
			return false
		}
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a != "version"
	}
	return true
}

// RenderCLIHeader is a one-shot stderr banner when the binary starts (TTY only).
func RenderCLIHeader(version string) string {
	line := GradientWord("ASSISTCLAW")
	sub := lipgloss.NewStyle().Foreground(ColorMuted).Render(
		"  edge neural shell  ·  " + strings.TrimSpace(version),
	)
	barW := 48
	if fd := int(os.Stderr.Fd()); term.IsTerminal(fd) {
		if tw, _, err := term.GetSize(fd); err == nil && tw > 12 {
			barW = tw - 4
			if barW > 72 {
				barW = 72
			}
		}
	}
	bar := lipgloss.NewStyle().Foreground(ColorBorder).Render(
		"  " + strings.Repeat("─", barW) + "▸",
	)
	return "\n" + line + "\n" + sub + "\n" + bar + "\n"
}

// MaybePrintCLIHeader prints a compact branded line to stderr when appropriate.
func MaybePrintCLIHeader(version string) {
	if ArgsWantNoColor() || os.Getenv("NO_COLOR") != "" {
		fmt.Fprintf(os.Stderr, "assistclaw %s\n", version)
		return
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintf(os.Stderr, "[assistclaw] version %s startup\n", version)
		return
	}
	fmt.Fprint(os.Stderr, RenderCLIHeader(version))
}

// MaybePrintVersion prints styled version info on a TTY, plain text otherwise.
func MaybePrintVersion(version string, noColor bool) {
	if noColor || ArgsWantNoColor() || os.Getenv("NO_COLOR") != "" {
		fmt.Printf("assistclaw %s\n", version)
		return
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Printf("assistclaw %s\n", version)
		return
	}
	fmt.Print(RenderVersionBlock(version))
}
