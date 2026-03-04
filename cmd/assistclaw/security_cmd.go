package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/security"
)

func securityCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Runtime security — guardrail status, audit log verification, and reporting",
		Long: `Security tools for AssistClaw:

  status   Show guardrail mode, log size, event count
  verify   Verify the audit log HMAC chain (detects tampering)
  tail     Stream live audit events
  report   Summary report by event type, tool, skill, and actor`,
	}
	cmd.AddCommand(
		securityStatusCmd(flags),
		securityVerifyCmd(flags),
		securityTailCmd(flags),
		securityReportCmd(flags),
	)
	return cmd
}

// securityLogPath returns the configured or default audit log path.
func securityLogPath(flags *globalFlags) (string, error) {
	log := buildLogger(flags.logLevel)
	cfg, err := loadConfig(flags.configPath, log)
	if err != nil {
		return "", err
	}
	if cfg.Security.LogPath != "" {
		return cfg.Security.LogPath, nil
	}
	return filepath.Join(cfg.StateDir, "security", "audit.ndjson"), nil
}

// ─────────────────────────────────────────────
// assistclaw security status
// ─────────────────────────────────────────────

func securityStatusCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current guardrail mode, audit log size, and last event",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(flags.logLevel)
			cfg, err := loadConfig(flags.configPath, log)
			if err != nil {
				return err
			}

			mode := cfg.Security.Mode
			if mode == "" {
				mode = string(security.ModeMonitor)
			}
			logPath := cfg.Security.LogPath
			if logPath == "" {
				logPath = filepath.Join(cfg.StateDir, "security", "audit.ndjson")
			}

			fmt.Printf("Security Layer Status\n")
			fmt.Printf("  Guardrail mode : %s\n", mode)
			fmt.Printf("  PII masking    : %v\n", cfg.Security.PIIMask)
			fmt.Printf("  Audit log      : %s\n", logPath)

			info, err := os.Stat(logPath)
			if os.IsNotExist(err) {
				fmt.Printf("  Log            : (no log yet — run the agent to create one)\n")
				return nil
			}
			if err != nil {
				return err
			}
			fmt.Printf("  Log size       : %s (%d bytes)\n", humanBytes(info.Size()), info.Size())
			fmt.Printf("  Log modified   : %s\n", info.ModTime().Format("2006-01-02 15:04:05 MST"))

			// Count events quickly
			f, err := os.Open(logPath)
			if err != nil {
				return err
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			count := 0
			for scanner.Scan() {
				if scanner.Text() != "" {
					count++
				}
			}
			fmt.Printf("  Total events   : %d\n", count)
			fmt.Printf("\nRun 'assistclaw security verify' to check chain integrity.\n")
			return nil
		},
	}
}

// ─────────────────────────────────────────────
// assistclaw security verify
// ─────────────────────────────────────────────

func securityVerifyCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify audit log HMAC chain integrity — detects any tampering or deletion",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := zap.NewNop()
			_ = log

			logPath, err := securityLogPath(flags)
			if err != nil {
				return err
			}

			fmt.Printf("Verifying audit log: %s\n", logPath)
			result, err := security.VerifyLog(logPath)
			if err != nil {
				return fmt.Errorf("verification failed: %w", err)
			}

			fmt.Print(security.FormatVerifyResult(logPath, result))
			if !result.OK() {
				os.Exit(1)
			}
			return nil
		},
	}
}

// ─────────────────────────────────────────────
// assistclaw security report
// ─────────────────────────────────────────────

func securityReportCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Summary report: events by type, top tools, skill reads, guardrail activity",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath, err := securityLogPath(flags)
			if err != nil {
				return err
			}
			report, err := security.SummaryReport(logPath)
			if err != nil {
				return err
			}
			fmt.Print(report)
			return nil
		},
	}
}

// ─────────────────────────────────────────────
// assistclaw security tail
// ─────────────────────────────────────────────

func securityTailCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "tail",
		Short: "Stream live audit events from the log (like tail -f, press Ctrl+C to stop)",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath, err := securityLogPath(flags)
			if err != nil {
				return err
			}

			fmt.Printf("Tailing: %s (Ctrl+C to stop)\n\n", logPath)

			f, err := os.Open(logPath)
			if os.IsNotExist(err) {
				fmt.Println("No audit log yet — start the agent to generate events.")
				return nil
			}
			if err != nil {
				return err
			}
			defer f.Close()

			// Seek to end — only show new events
			f.Seek(0, 2)

			for {
				buf := bufio.NewScanner(f)
				buf.Buffer(make([]byte, 1024*1024), 1024*1024)
				for buf.Scan() {
					line := buf.Text()
					if line != "" {
						fmt.Println(line)
					}
				}
				time.Sleep(500 * time.Millisecond)
			}
		},
	}
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func humanBytes(n int64) string {
	const kb, mb = 1024, 1024 * 1024
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
