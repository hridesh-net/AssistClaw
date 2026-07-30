package tui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ArgsWantNoColor returns true if the process args include a no-color flag
// (parsed before Cobra runs so the startup banner can honour it).
func ArgsWantNoColor() bool {
	for _, a := range os.Args[1:] {
		switch a {
		case "--no-color", "--no-colour":
			return true
		}
	}
	return false
}

// ShouldPrintStartupBanner is false for version/help invocations so stderr
// stays quiet when the user is just probing the binary.
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

// MaybePrintCLIHeader prints a compact branded line to stderr when the user
// has a colour-capable TTY; otherwise emits a plain text fallback.
func MaybePrintCLIHeader(version string) {
	if ArgsWantNoColor() || os.Getenv("NO_COLOR") != "" {
		fmt.Fprintf(os.Stderr, "assistclaw %s\n", version)
		return
	}
	fd := int(os.Stderr.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintf(os.Stderr, "[assistclaw] version %s startup\n", version)
		return
	}
	tw := 0
	if w, _, err := term.GetSize(fd); err == nil {
		tw = w
	}
	fmt.Fprint(os.Stderr, RenderCLIHeader(version, tw))
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
