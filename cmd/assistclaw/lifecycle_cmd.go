package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// start / stop / status / restart commands
// ─────────────────────────────────────────────

func startCmd(gf *globalFlags) *cobra.Command {
	var daemon bool
	var skipPreflight, preflightFull bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start AssistClaw in background mode",
		Long: `Starts AssistClaw with the gateway and messaging channels.

By default, runs a fast preflight (same checks as assistclaw doctor --skip-network) before binding ports. Use --preflight-full to probe LLM and channel APIs. Use --skip-preflight only if you trust this environment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execStart(gf, daemon, skipPreflight, preflightFull, cmd)
		},
	}
	cmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run detached in the background")
	registerPreflightFlags(cmd, &skipPreflight, &preflightFull)
	return cmd
}

func stopCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background AssistClaw process",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			pidFile := PidFile(cfg.StateDir)
			pid, err := ReadPID(pidFile)
			if err != nil {
				return fmt.Errorf("agent not running (no PID file)")
			}
			if !CheckPID(pid) {
				_ = os.Remove(pidFile)
				return fmt.Errorf("agent not running (stale PID file)")
			}
			process, _ := os.FindProcess(pid)
			if err := process.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("failed to stop agent: %w", err)
			}
			fmt.Printf("Stopping AssistClaw (PID: %d)...\n", pid)
			_ = os.Remove(pidFile)
			return nil
		},
	}
}

func statusCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check status, CPU, and RAM usage of AssistClaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			pid, err := ReadPID(PidFile(cfg.StateDir))
			if err != nil || !CheckPID(pid) {
				fmt.Println("● AssistClaw is NOT running.")
				fmt.Printf("  Start with: assistclaw start\n")
				return nil
			}

			// Count installed skills
			skillCount := 0
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			if entries, err := os.ReadDir(customDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						skillCount++
					}
				}
			}
			enabledCount := len(cfg.Agent.EnabledSkills)
			skillSummary := fmt.Sprintf("%d installed", skillCount)
			if enabledCount > 0 {
				skillSummary = fmt.Sprintf("%d enabled / %d installed", enabledCount, skillCount)
			}

			// Build channel list
			var channels []string
			if cfg.Channels.WhatsApp != nil {
				channels = append(channels, "WhatsApp")
			}
			if cfg.Channels.Telegram != nil {
				channels = append(channels, "Telegram")
			}
			if cfg.Channels.Discord != nil {
				channels = append(channels, "Discord")
			}
			if cfg.Channels.Slack != nil {
				channels = append(channels, "Slack")
			}

			// MCP transport
			mcpTransport := cfg.MCP.Server.Transport
			if mcpTransport == "" {
				mcpTransport = "stdio"
			}

			return tui.RunStatus(tui.StatusInfo{
				PID:           pid,
				Version:       version,
				SkillSummary:  skillSummary,
				Channels:      channels,
				PlanoEnabled:  cfg.Plano.Enabled,
				PlanoEndpoint: cfg.Plano.Endpoint,
				MCPEnabled:    cfg.MCP.Server.Enabled,
				MCPTransport:  mcpTransport,
				NoMouse:       gf.noMouse,
			})
		},
	}
}

func restartCmd(gf *globalFlags) *cobra.Command {
	var skipPreflight, preflightFull bool
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the background AssistClaw process",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = stopCmd(gf).RunE(cmd, args)
			time.Sleep(1 * time.Second)
			return execStart(gf, false, skipPreflight, preflightFull, cmd)
		},
	}
	registerPreflightFlags(cmd, &skipPreflight, &preflightFull)
	return cmd
}
