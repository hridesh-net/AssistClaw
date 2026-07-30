package main

import (
	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// version command
// ─────────────────────────────────────────────

func versionCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			tui.MaybePrintVersion(version, gf.noColor)
		},
	}
}
