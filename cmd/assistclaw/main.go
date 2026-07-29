// Package main is the AssistClaw CLI entry point.
// It builds the full dependency graph and dispatches to subcommands.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	chadapter "github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/awareness"
	"github.com/assistclaw/assistclaw/internal/automation"
	"github.com/assistclaw/assistclaw/internal/autotool"
	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/discord"
	"github.com/assistclaw/assistclaw/internal/channels/slack"
	"github.com/assistclaw/assistclaw/internal/channels/telegram"
	"github.com/assistclaw/assistclaw/internal/channels/whatsapp"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/crashreport"
	"github.com/assistclaw/assistclaw/internal/cron"
	"github.com/assistclaw/assistclaw/internal/email"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	embedproviders "github.com/assistclaw/assistclaw/internal/embeddings/providers"
	"github.com/assistclaw/assistclaw/internal/extensions"
	"github.com/assistclaw/assistclaw/internal/gateway"
	"github.com/assistclaw/assistclaw/internal/graph"
	"github.com/assistclaw/assistclaw/internal/localintel"
	"github.com/assistclaw/assistclaw/internal/mcp"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/mempalace"
	obstracing "github.com/assistclaw/assistclaw/internal/observability/tracing"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/provider/anthropic"
	"github.com/assistclaw/assistclaw/internal/provider/bedrock"
	"github.com/assistclaw/assistclaw/internal/provider/catalogs"
	"github.com/assistclaw/assistclaw/internal/provider/ollama"
	"github.com/assistclaw/assistclaw/internal/provider/openai"
	"github.com/assistclaw/assistclaw/internal/provider/openaicompat"
	planoprovider "github.com/assistclaw/assistclaw/internal/provider/plano"
	"github.com/assistclaw/assistclaw/internal/proactive"
	"github.com/assistclaw/assistclaw/internal/provider/vertex"
	"github.com/assistclaw/assistclaw/internal/security"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/assistclaw/assistclaw/internal/subagents"
	"github.com/assistclaw/assistclaw/internal/system"
	"github.com/assistclaw/assistclaw/internal/tools"
	"github.com/assistclaw/assistclaw/internal/voice"
	_ "github.com/assistclaw/assistclaw/internal/webui" // ensure embed FS is included
)

var version = "v3.10.31" // Overridden by -ldflags "-X main.version=..." during build

type reliableToolSender struct {
	rs *chadapter.ReliableSender
}

func (s reliableToolSender) SendText(ctx context.Context, sessionID, text string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session_id is required for outbound channel sends")
	}
	_, err := s.rs.Send(ctx, chadapter.OutboundMessage{
		SessionID: sessionID,
		Text:      text,
	})
	return err
}

// defaultHeartbeatPrompt matches AGENTS.md guidance for periodic heartbeat polls.
const defaultHeartbeatPrompt = `Read HEARTBEAT.md if it exists in your workspace (state directory). Follow it strictly. Do not infer or repeat old tasks from prior chats. If nothing needs attention, reply HEARTBEAT_OK.`

// runHeartbeatLoop issues periodic synthetic user turns on a dedicated session (logs only;
// use cron + channels if you need scheduled output delivered to Telegram/Slack).
func runHeartbeatLoop(ctx context.Context, base *agent.Runner, interval time.Duration, sessionID, prompt string, log *zap.Logger) {
	if interval < time.Minute {
		interval = time.Minute
	}
	if sessionID == "" {
		sessionID = "assistclaw:heartbeat"
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultHeartbeatPrompt
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	log.Info("heartbeat scheduler started",
		zap.String("session_id", sessionID),
		zap.Duration("interval", interval),
	)
	for {
		select {
		case <-ctx.Done():
			log.Info("heartbeat scheduler stopped")
			return
		case <-t.C:
			hbCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
			hr := base.WithSession(sessionID)
			_, err := hr.Run(hbCtx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: sessionID,
				Role:      memory.RoleUser,
				Content:   prompt,
				CreatedAt: time.Now(),
			})
			cancel()
			if err != nil {
				log.Warn("heartbeat tick failed", zap.Error(err))
			} else {
				log.Debug("heartbeat tick completed")
			}
		}
	}
}

// agentPlanningEnabled defaults to true when unset (upfront planning on).
func agentPlanningEnabled(c *config.Config) bool {
	if c.Agent.Planning == nil {
		return true
	}
	return *c.Agent.Planning
}

// agentReflectionEnabled defaults to false when unset (extra LLM call; opt-in).
func agentReflectionEnabled(c *config.Config) bool {
	if c.Agent.Reflection == nil {
		return false
	}
	return *c.Agent.Reflection
}

func main() {
	if tui.ShouldPrintStartupBanner() {
		tui.MaybePrintCLIHeader(version)
	}
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// Global flags
// ─────────────────────────────────────────────

type globalFlags struct {
	configPath              string
	logLevel                string
	noColor                 bool
	noMouse                 bool
	allowSensitiveSkills    []string
	allowAllSensitiveSkills bool
}

func rootCmd() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:   "assistclaw",
		Short: "AssistClaw — high-performance polyglot AI assistant",
		Long: `AssistClaw is a hardware-integrated AI assistant with:
  • 15+ LLM provider support (OpenAI, Anthropic, Bedrock, Vertex, Ollama, vLLM, and more)
  • Full embedding model support (remote + local)
  • Three-tier memory (Working → Episodic → Semantic/Vector)
  • Autonomous tool generation with safety sandboxing
  • Hardware sensing (Camera, Audio, GPIO)
  • Multi-channel messaging (Telegram, Discord, Slack, REST/WebSocket)`,
		Version:      version,
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVarP(&flags.configPath, "config", "c", "", "Config file path (default: ~/.assistclaw/assistclaw.yaml)")
	root.PersistentFlags().StringVar(&flags.logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	root.PersistentFlags().BoolVar(&flags.noColor, "no-color", false, "Disable color output")
	root.PersistentFlags().BoolVar(&flags.noMouse, "no-mouse", false, "Disable mouse capture in TUI (recommended inside tmux/screen)")
	root.PersistentFlags().StringSliceVar(&flags.allowSensitiveSkills, "allow-sensitive-skills", nil, "Allow named sensitive skills to execute (comma-separated)")
	root.PersistentFlags().BoolVar(&flags.allowAllSensitiveSkills, "allow-all-sensitive-skills", false, "Allow all sensitive skills to execute (use with care)")

	root.AddCommand(
		autoCmd(flags),
		agentCmd(flags),
		startCmd(flags),
		stopCmd(flags),
		statusCmd(flags),
		restartCmd(flags),
		providersCmd(flags),
		embeddingsCmd(flags),
		memoryCmd(flags),
		mempalaceCmd(flags),
		toolsCmd(flags),
		localIntelCmd(flags),
		extensionsCmd(flags),
		gatewayCmd(flags),
		onboardCmd(flags),
		skillsCmd(flags),
		mcpCmd(flags),
		serviceCmd(flags),
		securityCmd(flags),
		auditCmd(flags),
		logicTestCmd(flags),
		cronCmd(flags),
		dlqCmd(flags),
		proactiveCmd(flags),
		ruleCmd(flags),
		doctorCmd(flags),
		emailCmd(flags),
		goalCmd(flags),
		personaCmd(flags),
		versionCmd(flags),
		localgemmaCmd(flags),
	)
	return root
}

// ─────────────────────────────────────────────
// agent command
// ─────────────────────────────────────────────

func autoCmd(gf *globalFlags) *cobra.Command {
	var (
		model     string
		sessionID string
	)

	cmd := &cobra.Command{
		Use:   "auto [goal]",
		Short: "Start a continuous autonomous agent loop targeting a specific goal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(gf, gf.configPath, model, args[0], sessionID, false, false, true)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Model to use (e.g. anthropic/claude-3-5-sonnet)")
	cmd.Flags().StringVar(&sessionID, "session", "", "Resume an existing session by ID")
	return cmd
}

func agentCmd(gf *globalFlags) *cobra.Command {
	var (
		message   string
		model     string
		noStream  bool
		sessionID string
		serve     bool
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start an interactive agent session or send a single message",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(gf, gf.configPath, model, message, sessionID, serve, noStream, false)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Single message to send (non-interactive)")
	cmd.Flags().StringVar(&model, "model", "", "Model to use (e.g. anthropic/claude-haiku-3-5)")
	cmd.Flags().BoolVar(&noStream, "no-stream", false, "Disable streaming output")
	cmd.Flags().StringVar(&sessionID, "session", "", "Resume an existing session by ID")
	cmd.Flags().BoolVarP(&serve, "serve", "s", false, "Run in background mode with Gateway and messaging channels active")
	return cmd
}

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

// ─────────────────────────────────────────────
// providers command
// ─────────────────────────────────────────────

func providersCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "providers", Short: "List and manage LLM providers"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all configured LLM providers and their models",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := provider.NewRegistry()
			registerProviders(ctx, cfg, reg, log) //nolint:errcheck
			for _, p := range reg.All() {
				models, err := p.ListModels(ctx)
				suffix := ""
				if err != nil {
					suffix = " (error: " + err.Error() + ")"
				}
				fmt.Printf("\n%s%s\n", p.Name(), suffix)
				for _, m := range models {
					local := ""
					if m.Local {
						local = " [local]"
					}
					fmt.Printf("  - %s%s\n", m.ID, local)
				}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Check connectivity to all configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := provider.NewRegistry()
			registerProviders(ctx, cfg, reg, log) //nolint:errcheck
			report := reg.CheckAll(ctx)
			for name, result := range report.Results {
				status := "✓"
				detail := ""
				if !result.OK {
					status = "✗"
					detail = " — " + result.Error
				}
				fmt.Printf("%s %s%s\n", status, name, detail)
			}
			return nil
		},
	})
	return cmd
}

// ─────────────────────────────────────────────
// embeddings command
// ─────────────────────────────────────────────

func embeddingsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "embed", Short: "Embed text using configured embedding models"}
	cmd.AddCommand(&cobra.Command{
		Use:   "text [text]",
		Short: "Embed a piece of text and show the vector dimensions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			reg := embeddings.NewRegistry()
			registerEmbedders(ctx, cfg, reg, log)
			vec, err := reg.EmbedText(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Embedded %d-dimensional vector (showing first 8 dims): %v...\n", len(vec), vec[:min(8, len(vec))])
			return nil
		},
	})
	return cmd
}

// ─────────────────────────────────────────────
// memory command
// ─────────────────────────────────────────────

func memoryCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Search and manage conversation memory"}
	cmd.AddCommand(&cobra.Command{
		Use:   "search [query]",
		Short: "Full-text search of conversation history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			epMem, err := memory.NewEpisodicMemory(cfg.Memory.EpisodicDBPath)
			if err != nil {
				return err
			}
			defer epMem.Close()
			results, err := epMem.Search(ctx, args[0], 20)
			if err != nil {
				return err
			}
			for _, m := range results {
				fmt.Printf("[%s] %s: %s\n\n", m.CreatedAt.Format("2006-01-02 15:04"), m.Role, m.Content)
			}
			return nil
		},
	})
	mineCmd := &cobra.Command{Use: "mine", Short: "Mine/backfill taxonomy metadata for memory files"}
	var mineJSON bool
	var mineDryRun bool
	var mineMode string
	var mineLimit int
	var mineYes bool
	mineCmd.PersistentFlags().BoolVar(&mineJSON, "json", false, "Output JSON")
	mineCmd.PersistentFlags().BoolVar(&mineDryRun, "dry-run", false, "Plan run without indexing writes")
	mineCmd.PersistentFlags().StringVar(&mineMode, "mode", "", "Mining mode override: incremental|full")
	mineCmd.PersistentFlags().IntVar(&mineLimit, "limit", 0, "Maximum files to process")

	runMine := func(forceMode string) (*memory.MiningReport, error) {
		ctx := context.Background()
		log := buildLogger(gf.logLevel)
		cfg, err := loadConfig(gf.configPath, log)
		if err != nil {
			return nil, err
		}
		embedReg := embeddings.NewRegistry()
		registerEmbedders(ctx, cfg, embedReg, log)
		memMgr, err := memory.NewManager(memory.ManagerConfig{
			WorkingTokenBudget:  cfg.Memory.WorkingTokenBudget,
			EpisodicDBPath:      cfg.Memory.EpisodicDBPath,
			SemanticDBPath:      cfg.Memory.SemanticDBPath,
			EmbeddingDimensions: 1536,
			ChunkSize:           cfg.Memory.Mining.ChunkSize,
			ChunkOverlap:        cfg.Memory.Mining.ChunkOverlap,
		})
		if err != nil {
			return nil, err
		}
		defer memMgr.Close()
		mode := cfg.Memory.Mining.Mode
		if mineMode != "" {
			mode = mineMode
		}
		if forceMode != "" {
			mode = forceMode
		}
		maxFiles := cfg.Memory.Mining.MaxFilesPerRun
		if mineLimit > 0 {
			maxFiles = mineLimit
		}
		report, err := memMgr.Mine(ctx, embedReg, cfg.StateDir, memory.MiningOptions{
			Mode:           mode,
			Include:        cfg.Memory.Mining.Include,
			Exclude:        cfg.Memory.Mining.Exclude,
			MaxFilesPerRun: maxFiles,
			MaxFileSizeKB:  cfg.Memory.Mining.MaxFileSizeKB,
			StatePath:      cfg.Memory.Mining.StatePath,
			DryRun:         mineDryRun,
		})
		if err != nil {
			return nil, err
		}
		return &report, nil
	}

	mineCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run an incremental mining pass",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := runMine("")
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine run complete: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "backfill",
		Short: "Run a full backfill over all included files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !mineYes {
				return fmt.Errorf("backfill requires --yes")
			}
			report, err := runMine("full")
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine backfill complete: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show last mining run status",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			report, err := memory.ReadMiningState(cfg.Memory.Mining.StatePath)
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine status: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate mining config and embedder readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			embedReg := embeddings.NewRegistry()
			registerEmbedders(context.Background(), cfg, embedReg, log)
			if _, ok := embedReg.Default(); !ok {
				return fmt.Errorf("no embedding provider available for mining")
			}
			if mineJSON {
				fmt.Println(`{"schema_version":1,"valid":true}`)
				return nil
			}
			fmt.Println("memory mine validate: ok")
			return nil
		},
	})
	mineCmd.PersistentFlags().BoolVar(&mineYes, "yes", false, "Confirm destructive/full backfill operations")
	cmd.AddCommand(mineCmd)
	return cmd
}

// ─────────────────────────────────────────────
// tools command
// ─────────────────────────────────────────────

func toolsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Short: "List all tools available to the agent"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all tools: built-in, skill, and auto-generated",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			path := gf.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			cfg, err := loadConfig(path, log)
			if err != nil {
				return err
			}

			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			header := func(s string) string { return tui.Style(tui.ColorNeon, true, s) }

			// ── Section 1: Built-in tools ──────────────────────────────────────
			fmt.Println(header("\n⚡ Built-in System Tools") + dim("  (always available)"))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			builtins := []struct{ name, desc string }{
				{"bash", "Execute any shell command (mkdir, git, npm, pip, compile, run tests…)"},
				{"write_file", "Create or overwrite any file — source code, configs, scripts"},
				{"read_file", "Read file contents (with optional line range)"},
				{"list_dir", "Browse directory contents, optionally recursive"},
				{"grep", "Search patterns across files (regex, case-insensitive modes)"},
				{"web_fetch", "Fetch text content from a URL (docs, APIs, READMEs)"},
				{"browser_navigate", "Open a URL in a real browser session"},
				{"browser_screenshot", "Capture a screenshot of the browser"},
				{"memory_search", "Search episodic + semantic (vector) memory"},
				{"memory_get", "Read a specific line range from a memory file"},
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, t := range builtins {
				fmt.Fprintf(w, "  %s\t%s\n", prim(t.name), dim(t.desc))
			}
			w.Flush()

			// ── Section 2: Skill tools ─────────────────────────────────────────
			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			skillReg := skills.NewRegistry()
			_ = skillReg.LoadAll(context.Background(), customDir)
			allSkills := skillReg.List()

			var skillToolCount int
			for _, s := range allSkills {
				skillToolCount += len(s.Tools)
			}

			fmt.Println(header("\n🧠 Skill Tools") + dim(fmt.Sprintf("  (%d installed skills)", len(allSkills))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			if skillToolCount == 0 {
				fmt.Println(dim("  No skill tools installed yet."))
				fmt.Println(dim("  Run: assistclaw skills install <name>"))
			} else {
				w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, s := range allSkills {
					for _, t := range s.Tools {
						fmt.Fprintf(w2, "  %s\t%s\t%s\n",
							prim(t.Name),
							dim("["+s.Name+"]"),
							dim(t.Description))
					}
				}
				w2.Flush()
			}

			// ── Section 3: Auto-generated tools ───────────────────────────────
			creator, err := autotool.NewCreator(autotool.CreatorConfig{
				ToolsDir: cfg.Agent.ToolsDir,
				VenvPath: filepath.Join(cfg.StateDir, "venv"),
				Timeout:  30,
			}, log)
			var autoList []autotool.ToolMeta
			if err == nil {
				autoList, _ = creator.List()
			}

			fmt.Println(header("\n🔧 Auto-generated Tools") + dim(fmt.Sprintf("  (%d generated)", len(autoList))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			if len(autoList) == 0 {
				fmt.Println(dim("  No auto-generated tools yet."))
				fmt.Println(dim("  Ask the agent to create one — it uses 'bash' and 'write_file' automatically."))
			} else {
				w3 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, t := range autoList {
					fmt.Fprintf(w3, "  %s\t%s\t%s\n",
						prim(t.Name),
						dim(t.CreatedAt.Format("2006-01-02")),
						dim(t.Description))
				}
				w3.Flush()
			}

			fmt.Println()
			return nil
		},
	})
	return cmd
}

func extensionsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extensions",
		Short: "Extension hooks and built-in equivalents (AssistClaw)",
		Long: `AssistClaw does not load third-party Node extension bundles. Built-in coverage:

  • Channels, skills, MCP clients, webhooks, cron, browser tools, voice — see list.
  • Optional prompt_files in assistclaw.yaml merge markdown into the system prompt
    (workspace prompt fragments as plain markdown, not executable plugins).

Extend behavior via skills, MCP, channels, or prompt_files as needed.`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Summarize extension-equivalent features and extensions.* config",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }

			fmt.Println(prim("Channels (in-process, not npm plugins)"))
			var ch []string
			if cfg.Channels.WhatsApp != nil {
				ch = append(ch, "whatsapp")
			}
			if cfg.Channels.Telegram != nil {
				ch = append(ch, "telegram")
			}
			if cfg.Channels.Discord != nil {
				ch = append(ch, "discord")
			}
			if cfg.Channels.Slack != nil {
				ch = append(ch, "slack")
			}
			if len(ch) == 0 {
				fmt.Println(dim("  (none configured)"))
			} else {
				fmt.Println(dim("  " + strings.Join(ch, ", ")))
			}

			fmt.Println(prim("\nMCP"))
			fmt.Printf("  server enabled: %v  transport: %s\n", cfg.MCP.Server.Enabled, cfg.MCP.Server.Transport)
			fmt.Printf("  external clients: %d\n", len(cfg.MCP.Clients))

			fmt.Println(prim("\nWebhooks"))
			if cfg.Webhooks.Enabled {
				fmt.Printf("  enabled, mappings: %d\n", len(cfg.Webhooks.Mappings))
			} else {
				fmt.Println(dim("  disabled"))
			}

			fmt.Println(prim("\nCron"))
			fmt.Printf("  jobs: %d\n", len(cfg.Cron))

			fmt.Println(prim("\nSkills"))
			fmt.Printf("  enabled in config: %d\n", len(cfg.Agent.EnabledSkills))

			fmt.Println(prim("\nVoice / browser / memory"))
			fmt.Println(dim("  voice: STT/TTS via internal/voice (yaml voice:)"))
			fmt.Println(dim("  browser: browser_navigate, browser_screenshot (chromedp)"))
			fmt.Println(dim("  memory: working + episodic.db + semantic (embeddings); optional MemPalace via mcp / memory.mempalace (managed_venv automates pip + init)"))

			fmt.Println(prim("\nextensions.prompt_files (extra markdown prompt fragments)"))
			if !cfg.Extensions.Enabled {
				fmt.Println(dim("  disabled (set extensions.enabled: true)"))
			} else if len(cfg.Extensions.PromptFiles) == 0 {
				fmt.Println(dim("  enabled, no files listed"))
			} else {
				for _, p := range cfg.Extensions.PromptFiles {
					fmt.Printf("  • %s\n", p)
				}
			}
			fmt.Println()
			return nil
		},
	})
	return cmd
}

// ─────────────────────────────────────────────
// gateway command
// ─────────────────────────────────────────────

// ─────────────────────────────────────────────
// gateway command (start · stop · restart · serve)
// ─────────────────────────────────────────────

func gatewayCmd(gf *globalFlags) *cobra.Command {
	var skipPreflight, preflightFull bool
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Manage the AssistClaw Gateway and Web UI",
		Long: `Manage the AssistClaw background gateway and embedded web UI.

Subcommands:
  start    Start in background daemon mode (web UI + channels)
  stop     Stop the running background daemon
  restart  Restart the background daemon
  serve    Run the gateway in the foreground (blocks terminal)
  status   Show daemon status (alias of 'assistclaw status')

By default, start and serve run a fast preflight (doctor subset, --skip-network) before binding. Use --preflight-full for full network checks.`,
	}
	registerGatewayPreflightFlags(cmd, &skipPreflight, &preflightFull)

	// gateway start — alias of 'assistclaw start --daemon'
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start AssistClaw daemon in background (web UI + agent + channels)",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			pctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
			defer cancel()
			if err := runPreflight(pctx, gf, preflightOpts{Skip: skipPreflight, Full: preflightFull}, log, cmd.ErrOrStderr()); err != nil {
				return err
			}
			return Detach("start")
		},
	})

	// gateway stop — alias of 'assistclaw stop'
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the running AssistClaw background daemon",
		RunE:  stopCmd(gf).RunE,
	})

	// gateway restart — alias of 'assistclaw restart'
	cmd.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart the AssistClaw background daemon",
		RunE:  restartCmd(gf).RunE,
	})

	// gateway status — alias of 'assistclaw status'
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show AssistClaw daemon status, PID, and web UI address",
		RunE:  statusCmd(gf).RunE,
	})

	// gateway serve — foreground gateway-only server (dev/debug)
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the gateway in the foreground (blocks terminal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck

			pctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
			defer cancel()
			if err := runPreflight(pctx, gf, preflightOpts{Skip: skipPreflight, Full: preflightFull}, log, cmd.ErrOrStderr()); err != nil {
				return err
			}

			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			log.Info("Starting AssistClaw Gateway (foreground)...",
				zap.String("host", cfg.Gateway.Host),
				zap.Int("port", cfg.Gateway.Port),
				zap.String("bind", cfg.Gateway.Bind),
			)
			srv := gateway.NewServer(cfg.Gateway.Port)
			srv.Bind = cfg.Gateway.Bind
			srv.Tailscale.Mode = cfg.Gateway.Tailscale.Mode
			srv.Token = cfg.Gateway.Token
			srv.Version = version
			srv.Config = cfg
			srv.Logger = log

			if cfg.Gmail.Enabled {
				srv.Gmail = automation.NewGmailWatcher(cfg.Gmail, log)
			}
			if cfg.Voice.Enabled {
				srv.Voice = voice.NewDaemon(cfg.Voice)
			}

			webHost := cfg.Gateway.Host
			if webHost == "" {
				webHost = "localhost"
			}
			fmt.Printf("\n🌐 Web UI: http://%s:%d\n", webHost, cfg.Gateway.Port)

			errCh := make(chan error, 1)
			go func() {
				if err := srv.Start(context.Background()); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			select {
			case err := <-errCh:
				return fmt.Errorf("gateway error: %w", err)
			case <-sigCh:
				log.Info("Shutting down Gateway...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Stop(ctx); err != nil {
					return fmt.Errorf("shutdown error: %w", err)
				}
			}
			return nil
		},
	})

	return cmd
}

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

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// effectiveMCPClients returns cfg.MCP.Clients plus an optional synthetic MemPalace stdio client
// when memory.mempalace.auto_start is true and no client with the same name is already defined.
func effectiveMCPClients(cfg *config.Config) ([]config.MCPClientConfig, bool) {
	out := append([]config.MCPClientConfig(nil), cfg.MCP.Clients...)
	name := strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)
	if name == "" {
		name = "mempalace"
	}
	if !cfg.Memory.MemPalace.AutoStart {
		return out, false
	}
	for _, c := range out {
		if c.Name == name {
			return out, false
		}
	}
	py := strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable)
	if py == "" {
		py = "python3"
	}
	syn := config.MCPClientConfig{
		Name:      name,
		Transport: "stdio",
		Command:   py,
		Args:      []string{"-m", "mempalace.mcp_server"},
	}
	if cfg.Memory.MemPalace.ManagedVenv {
		syn.Dir = mempalace.ManagedWorldDir(cfg.StateDir)
	}
	out = append(out, syn)
	return out, true
}

// augmentActiveSkillsWithMCP appends skill names registered by external MCP (prefix "mcp:")
// so they appear in the session skills header and skill_graph_index without requiring users
// to list every server in agent.enabled_skills.
func augmentActiveSkillsWithMCP(skillReg skills.Registry, active []string) []string {
	seen := make(map[string]struct{}, len(active))
	for _, n := range active {
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
	}
	out := append([]string(nil), active...)
	for _, s := range skillReg.List() {
		name := s.Name
		if !strings.HasPrefix(name, "mcp:") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func mcpClientConfigsFromYAML(in []config.MCPClientConfig) []mcp.ClientConfig {
	out := make([]mcp.ClientConfig, 0, len(in))
	for _, c := range in {
		tr := mcp.TransportStdio
		if strings.EqualFold(strings.TrimSpace(c.Transport), "http") {
			tr = mcp.TransportHTTP
		}
		out = append(out, mcp.ClientConfig{
			Name:      c.Name,
			Transport: tr,
			Command:   c.Command,
			Args:      c.Args,
			Dir:       c.Dir,
			Env:       c.Env,
			URL:       c.URL,
			AuthToken: c.AuthToken,
		})
	}
	return out
}

func loadConfig(path string, log *zap.Logger) (*config.Config, error) {
	if path == "" {
		path = config.DefaultConfigPath()
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := runOnboarding(context.Background(), path); err != nil {
			log.Warn("interactive onboarding failed or was skipped, falling back to environment variables", zap.Error(err))
			return config.LoadFromEnv(), nil
		}
	}
	return config.Load(path)
}

func runAgent(gf *globalFlags, configPath string, model string, message string, sessionID string, serve bool, noStream bool, auto bool) (err error) {
	// Panic recovery: write crash report and re-panic so systemd restarts.
	defer func() {
		if r := recover(); r != nil {
			// cfg may not be loaded yet; try to use configPath to derive state dir.
			stateDir := configPath
			if stateDir == "" {
				stateDir = os.Getenv("HOME")
			}
			crashreport.Recover(stateDir, version, buildLogger(gf.logLevel))
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := buildLogger(gf.logLevel)
	defer log.Sync() //nolint:errcheck

	cfg, err := loadConfig(configPath, log)
	if err != nil {
		return err
	}

	shutdownTracing := obstracing.Init(ctx, obstracing.Config{
		Enabled:     cfg.Tracing.Enabled,
		Endpoint:    cfg.Tracing.OTLPEndpoint,
		ServiceName: cfg.Tracing.ServiceName,
		SampleRatio: cfg.Tracing.SampleRatio,
	}, log)
	defer func() {
		_ = shutdownTracing(context.Background())
	}()

	// Seed workspace identity files (SOUL.md, IDENTITY.md, AGENTS.md, etc.)
	// Use cfg.StateDir (already resolved to ~/.assistclaw) not the raw configPath
	// flag which can be an empty string when no --config flag is passed.
	resolvedConfigPath := filepath.Join(cfg.StateDir, "assistclaw.yaml")
	if wsErr := config.InitializeWorkspace(resolvedConfigPath); wsErr != nil {
		log.Warn("workspace init failed", zap.Error(wsErr))
	}

	// Boot all subsystems
	reg := provider.NewRegistry()
	if err := registerProviders(ctx, cfg, reg, log); err != nil {
		log.Warn("some providers failed to register", zap.Error(err))
	}

	embedReg := embeddings.NewRegistry()
	registerEmbedders(ctx, cfg, embedReg, log)

	// Ensure memory dirs exist
	if err := os.MkdirAll(filepath.Dir(cfg.Memory.EpisodicDBPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Memory.SemanticDBPath), 0o755); err != nil {
		return err
	}
	dims := 1536 // default for OpenAI small
	if e, ok := embedReg.Default(); ok {
		models, _ := e.ListModels(ctx)
		if len(models) > 0 {
			dims = models[0].Dimensions
		}
	}
	memMgr, err := memory.NewManager(memory.ManagerConfig{
		WorkingTokenBudget:  cfg.Memory.WorkingTokenBudget,
		EpisodicDBPath:      cfg.Memory.EpisodicDBPath,
		SemanticDBPath:      cfg.Memory.SemanticDBPath,
		EmbeddingDimensions: dims,
		ChunkSize:           cfg.Memory.Mining.ChunkSize,
		ChunkOverlap:        cfg.Memory.Mining.ChunkOverlap,
	})
	if err != nil {
		return fmt.Errorf("memory init: %w", err)
	}
	defer memMgr.Close()

	// Start Markdown memory watcher/synchronizer
	go func() {
		if err := memMgr.Watch(ctx, embedReg, cfg.StateDir); err != nil {
			log.Warn("memory watcher failed", zap.Error(err))
		}
	}()

	// Resolve model
	resolvedModel := model
	if resolvedModel == "" {
		resolvedModel = cfg.Routing.Default
	}
	if resolvedModel == "" {
		// Auto-select first available provider
		for _, p := range reg.All() {
			models, _ := p.ListModels(ctx)
			if len(models) > 0 {
				resolvedModel = p.Name() + "/" + models[0].ID
				break
			}
		}
	}
	if resolvedModel == "" {
		return fmt.Errorf("no model configured — set routing.default in config or use --model")
	}

	p, modelInfo, err := reg.ResolveModel(ctx, resolvedModel)
	if err != nil {
		return fmt.Errorf("resolve model %q: %w", resolvedModel, err)
	}
	log.Info("using model", zap.String("model", modelInfo.ID), zap.String("provider", p.Name()))

	// Extract bundled skills on first run
	bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")
	customDir := filepath.Join(cfg.StateDir, "skills", "custom")
	if err := extractBundledSkills(bundledDir); err != nil {
		log.Warn("failed to extract bundled skills", zap.Error(err))
	}

	// Load Skills — respects enabled_skills if set, else loads all from custom dir
	skillReg := skills.NewRegistry()
	if err := skillReg.LoadAll(ctx, customDir); err != nil {
		log.Warn("failed to load skills", zap.Error(err))
	}

	// Determine active skill names
	activeSkillNames := cfg.Agent.EnabledSkills
	if len(activeSkillNames) == 0 {
		// No explicit list = all installed custom skills are active
		for _, s := range skillReg.List() {
			activeSkillNames = append(activeSkillNames, s.Name)
		}
	}

	// Build tool registry
	toolReg := agent.NewToolRegistry()

	// Build the sensitive-skill approver: union of YAML allow-list, CLI
	// allow-list, and the "trust everything" CLI escape hatch. Sensitive
	// skills not in this set will refuse to execute at call time.
	allowedSensitive := map[string]bool{}
	for _, n := range cfg.Agent.EnabledSensitiveSkills {
		allowedSensitive[strings.TrimSpace(n)] = true
	}
	for _, n := range gf.allowSensitiveSkills {
		allowedSensitive[strings.TrimSpace(n)] = true
	}
	allowAllSensitive := gf.allowAllSensitiveSkills
	approver := func(skillName, _ string) bool {
		return allowAllSensitive || allowedSensitive[skillName]
	}

	// Register tools from skills
	for _, s := range skillReg.List() {
		skillTools := skills.ConvertTools(&s, cfg.Agent.SkillsDir, approver) // Simplification: assuming all tools relative to skills dir
		for _, t := range skillTools {
			toolReg.Register(t)
		}
	}

	if cfg.Memory.MemPalace.ManagedVenv && cfg.Memory.MemPalace.AutoStart {
		log.Info("mempalace: ensuring managed venv (first run may download PyPI packages)")
		if err := mempalace.Ensure(ctx, mempalace.EnsureOptions{
			StateDir:        cfg.StateDir,
			BootstrapPython: cfg.Memory.MemPalace.BootstrapPython,
			Progress:        os.Stderr,
			Log:             log,
		}); err != nil {
			return fmt.Errorf("mempalace managed venv: %w", err)
		}
	}

	mcpClientList, mempalaceAuto := effectiveMCPClients(cfg)
	if mempalaceAuto {
		msg := "mcp: MemPalace auto-start (stdio child process)"
		if cfg.Memory.MemPalace.ManagedVenv {
			msg = "mcp: MemPalace auto-start (managed venv + stdio child)"
		}
		log.Info(msg,
			zap.String("client", strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)),
			zap.String("python", strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable)),
		)
	}
	mcpCfgs := mcpClientConfigsFromYAML(mcpClientList)
	var externalMCP []*mcp.Client
	if len(mcpCfgs) > 0 {
		externalMCP = mcp.RegisterExternalMCPTools(ctx, mcpCfgs, skillReg, toolReg, nil, log)
	}
	defer func() {
		for i := len(externalMCP) - 1; i >= 0; i-- {
			_ = externalMCP[i].Close()
		}
	}()

	// Virtual MCP skills are registered after the initial activeSkillNames pass; include them so
	// BuildContext, skill_graph_index, and read_skill_node see custom MCP servers alongside disk skills.
	activeSkillNames = augmentActiveSkillsWithMCP(skillReg, activeSkillNames)
	skillsCtx := skillReg.BuildContext(activeSkillNames)

	memSearchFn := func(searchCtx context.Context, query string, limit int) ([]string, error) {
		var out []string
		router := memory.NewHeuristicPalaceRouter()
		route := router.Route(query)
		shouldRoute := cfg.Agent.Palace.Enabled && !cfg.Agent.Palace.ShadowOnly && cfg.Agent.Palace.MemorySearchRouting

		// 1. Episodic Search (Full-Text)
		msgs, err := memMgr.Episodic.Search(searchCtx, query, limit)
		if err == nil {
			for _, m := range msgs {
				out = append(out, fmt.Sprintf("[episodic] [%s] %s: %s", m.CreatedAt.Format("2006-01-02 15:04"), m.Role, m.Content))
			}
		}

		// 2. Semantic Search (Vector)
		if vec, err := embedReg.EmbedQuery(searchCtx, query); err == nil {
			docs, err := memMgr.Semantic.SearchWithModel(searchCtx, vec, limit)
			if err == nil {
				if shouldRoute && !route.IsZero() {
					filtered := make([]memory.Document, 0, len(docs))
					for _, d := range docs {
						if route.MatchesDocument(d) {
							filtered = append(filtered, d)
						}
					}
					if len(filtered) > 0 {
						docs = filtered
					} else if !cfg.Agent.Palace.FailOpen {
						docs = nil
					}
				}
				var docPaths []string
				for _, d := range docs {
					docPaths = append(docPaths, d.Source)
					out = append(out, fmt.Sprintf("[semantic] [score:%.2f] [%s / %s] source=%s taxonomy=%s/%s/%s: %s", d.Score, d.Model, d.CreatedAt.Format("2006-01-02 15:04"), d.Source, d.Palace, d.Wing, d.Room, d.Content))
				}

				// QueryWeaver Logic: Discover bridges between matched skill nodes
				if bridges := skillReg.FindBridges(docPaths); len(bridges) > 0 {
					for _, b := range bridges {
						out = append(out, fmt.Sprintf("[semantic] [bridge] source=%s: %s", b.FilePath, b.Instructions))
					}
				}
			}
		}

		if cfg.Memory.MemPalace.Enabled && cfg.Memory.MemPalace.InjectIntoMemorySearch {
			clientName := strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)
			if clientName == "" {
				clientName = "mempalace"
			}
			toolName := "mcp:" + clientName + ":mempalace_search"
			if t, ok := toolReg.Get(toolName); ok {
				mpLimit := limit
				if cfg.Memory.MemPalace.SearchLimit > 0 {
					mpLimit = cfg.Memory.MemPalace.SearchLimit
				}
				input, err := json.Marshal(map[string]any{
					"query": query,
					"limit": mpLimit,
				})
				if err == nil {
					if text, err := t.Execute(searchCtx, input); err != nil {
						log.Debug("mempalace_search delegate failed", zap.String("tool", toolName), zap.Error(err))
					} else if strings.TrimSpace(text) != "" {
						out = append(out, "[mempalace] "+text)
					}
				}
			} else {
				log.Warn("memory.mempalace.inject_into_memory_search is true but MCP tool is missing",
					zap.String("expected_tool", toolName),
					zap.String("hint", "enable memory.mempalace.managed_venv + auto_start, or run: assistclaw mempalace setup"),
				)
			}
		}

		return out, nil
	}
	memSnippetFn := func(snippetCtx context.Context, source string, startLine, endLine int) (string, error) {
		// If source doesn't exist, try resolving it relative to workspace
		path := source
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = filepath.Join(cfg.StateDir, source) // cfg.StateDir is workspace dir for now
		}
		return memMgr.Semantic.GetSnippet(snippetCtx, path, startLine, endLine)
	}
	channelSenders := map[string]tools.ChannelSender{}
	for _, t := range tools.Default(memSearchFn, memSnippetFn, memMgr.Episodic, p, modelInfo.ID, channelSenders, cfg.StateDir) {
		if tool, ok := t.(agent.Tool); ok {
			toolReg.Register(tool)
		}
	}
	// Build the graph-based tool catalog for per-request token-efficient selection.
	toolGraph := graph.NewToolGraph()
	catalog := tools.NewCatalog(toolReg, toolGraph)

	// Register find_tools (the Anthropic-pattern tool discovery tool).
	toolReg.Register(tools.FindToolsTool{Catalog: catalog})

	// Register skill_graph_index (on-demand skill node discovery).
	// activeSkillNames is already populated above (cfg.Agent.EnabledSkills or all loaded skills).
	toolReg.Register(&skills.SkillGraphIndexTool{
		Registry:     skillReg,
		ActiveSkills: activeSkillNames,
	})

	// Also register read_skill_node here (previously registered separately).
	toolReg.Register(skills.NewReadSkillNodeTool(skillReg))

	// Register repair_skill (auto-installation of missing dependencies).
	toolReg.Register(&skills.RepairSkillTool{Registry: skillReg})

	// Proactive self-healing: repair all enabled skills if they have missing dependencies.
	_ = skillReg.RepairAllEnabled(ctx, activeSkillNames)

	// Rebuild catalog to include all newly registered tools (find_tools, skill_graph_index, read_skill_node).
	catalog = tools.NewCatalog(toolReg, toolGraph)

	// Derive provider name for capability detection from the resolved model ID.
	// e.g. "anthropic/claude-opus-4" → "anthropic", "claude-opus-4" → "" (uses default caps)
	providerNameForCaps := ""
	if modelInfo.ID != "" {
		if idx := strings.Index(modelInfo.ID, "/"); idx > 0 {
			providerNameForCaps = strings.ToLower(modelInfo.ID[:idx])
		}
	}

	hw, _ := system.Detect(ctx)
	extPrompt := ""
	if cfg.Extensions.Enabled && len(cfg.Extensions.PromptFiles) > 0 {
		extPrompt = extensions.PromptAppendix(cfg.StateDir, cfg.Extensions.PromptFiles)
	}
	localIntelCache := strings.TrimSpace(cfg.Agent.LocalIntel.CacheDir)
	if localIntelCache == "" {
		localIntelCache = filepath.Join(cfg.StateDir, "localintel")
	}

	// Wrap all registered providers in a failover layer with circuit breakers.
	// The originally resolved provider becomes the first preferred primary.
	var primaries []provider.Provider
	primaries = append(primaries, p)
	for _, rp := range reg.All() {
		if rp.Name() != p.Name() {
			primaries = append(primaries, rp)
		}
	}
	// Build local gemma fallback if configured.
	var fallback provider.Provider
	if cfg.Agent.LocalIntel.Enabled && cfg.Agent.LocalIntel.GGUFPath != "" {
		liEng, liErr := localintel.Open(localintel.Options{
			CacheDir: localIntelCache,
			GGUFPath: cfg.Agent.LocalIntel.GGUFPath,
		})
		if liErr == nil && liEng.Available() {
			fallback = provider.NewLocalIntelProvider(liEng, "local/gemma", cfg.Agent.LocalIntel.SystemPrompt, cfg.Agent.LocalIntel.MaxTokens)
			log.Info("local intel fallback available", zap.String("model", "local/gemma"))
		} else if liErr != nil {
			log.Warn("local intel fallback unavailable", zap.Error(liErr))
		}
	}
	if len(primaries) > 1 || fallback != nil {
		p = provider.NewFailoverProvider(primaries, fallback, log)
		log.Info("provider failover enabled", zap.Int("primaries", len(primaries)), zap.Bool("fallback", fallback != nil))
	}

	runner := agent.NewRunner(agent.Config{
		MaxIterations:         cfg.Agent.MaxIterations,
		Model:                 modelInfo.ID,
		ActiveSkillsContext:   skillsCtx,
		ProviderName:          providerNameForCaps,
		ToolsProfile:          cfg.Security.Profile,
		EnablePlanning:        agentPlanningEnabled(cfg),
		EnableReflection:      agentReflectionEnabled(cfg),
		GatewayPublicBaseURL:  cfg.PublicGatewayBaseURL(),
		ExtensionPromptAppend: extPrompt,
		StateDir:              cfg.StateDir,
		LocalIntel: agent.LocalIntelRunnerConfig{
			Enabled:      cfg.Agent.LocalIntel.Enabled,
			GGUFPath:     cfg.Agent.LocalIntel.GGUFPath,
			MaxTokens:    cfg.Agent.LocalIntel.MaxTokens,
			SystemPrompt: cfg.Agent.LocalIntel.SystemPrompt,
			CacheDir:     localIntelCache,
		},
		Palace: agent.PalaceConfig{
			Enabled:             cfg.Agent.Palace.Enabled,
			ShadowOnly:          cfg.Agent.Palace.ShadowOnly,
			PromptRouting:       cfg.Agent.Palace.PromptRouting,
			MemorySearchRouting: cfg.Agent.Palace.MemorySearchRouting,
			ToolRouting:         cfg.Agent.Palace.ToolRouting,
			FailOpen:            cfg.Agent.Palace.FailOpen,
			LogDecisions:        cfg.Agent.Palace.LogDecisions,
		},
	}, p, toolReg, memMgr, log, cfg.StateDir).WithCatalog(catalog).WithHardware(hw)

	// ── Security: Guardrail + Audit Log ────────────────────────────────
	guardrailMode := security.GuardrailMode(cfg.Security.Mode)
	if guardrailMode == "" {
		guardrailMode = security.ModeMonitor
	}
	var ownerOnly []string
	if cfg.Security.OwnerOnlyPaths != nil {
		ownerOnly = *cfg.Security.OwnerOnlyPaths
	}
	guardrail, guardErr := security.NewGuardrail(guardrailMode, cfg.Security.BlockPatterns, ownerOnly)
	if guardErr == nil && len(cfg.Security.UserDenyPaths) > 0 {
		guardrail = guardrail.WithUserDenyPaths(cfg.Security.UserDenyPaths)
	}
	if guardErr != nil {
		log.Warn("security guardrail init failed", zap.Error(guardErr))
	}

	secLogPath := cfg.Security.LogPath
	if secLogPath == "" {
		secLogPath = filepath.Join(cfg.StateDir, "security", "audit.ndjson")
	}
	auditLog, auditErr := security.NewAuditLog(secLogPath, cfg.Security.PIIMask, log)
	if auditErr != nil {
		log.Warn("security audit log init failed", zap.Error(auditErr))
	}

	runner = runner.WithSecurity(guardrail, auditLog)
	log.Info("security layer active",
		zap.String("mode", string(guardrailMode)),
		zap.String("audit_log", secLogPath),
	)

	// Sub-agents (delegation): register after security so child runs inherit guardrail/audit.
	subSvc := &tools.SubAgentSvc{
		Store:                 subagents.NewStore(cfg.StateDir),
		Provider:              p,
		ParentRegistry:        toolReg,
		ToolGraph:             toolGraph,
		Mem:                   memMgr,
		Log:                   log,
		Model:                 modelInfo.ID,
		ActiveSkillsContext:   skillsCtx,
		ProviderName:          providerNameForCaps,
		GatewayPublicBaseURL:  cfg.PublicGatewayBaseURL(),
		ExtensionPromptAppend: extPrompt,
		DefaultToolsProfile:   cfg.Security.Profile,
		Guardrail:             guardrail,
		AuditLog:              auditLog,
		Hardware:              hw,
	}
	toolReg.Register(tools.SubAgentCreateTool{S: subSvc})
	toolReg.Register(tools.SubAgentListTool{S: subSvc})
	toolReg.Register(tools.SubAgentRunTool{S: subSvc})
	toolReg.Register(tools.SubAgentRemoveTool{S: subSvc})
	catalog = tools.NewCatalog(toolReg, toolGraph)
	runner = runner.WithCatalog(catalog).WithModelRegistry(reg)

	// ── Awareness (live context: time of day, presence, calendar) ──────
	awareStore := awareness.NewStore(cfg.StateDir)
	awareness.StartIdlePoller(ctx, awareStore, time.Minute)
	runner = runner.WithAwareness(awareStore)

	// ── Cron Daemon ───────────────────────────────────────────────────
	var cronJobs []cron.Job
	for _, j := range cfg.Cron {
		cronJobs = append(cronJobs, cron.Job{
			ID:         j.ID,
			Schedule:   j.Schedule,
			Prompt:     j.Prompt,
			MaxRetries: j.MaxRetries,
		})
	}

	cronDaemon := cron.NewDaemon(
		cronJobs,
		runner,
		log,
		filepath.Join(cfg.StateDir, "cron_jobs.json"),
	).WithFailureNotifier(func(ctx context.Context, jobID, summary string) {
		// Default failure path: structured log + episodic memory note so
		// the user discovers it on next interactive session. Channel-side
		// notifications are layered on top by the gateway when available.
		log.Error("cron failure notification", zap.String("id", jobID), zap.String("summary", summary))
		if memMgr != nil && memMgr.Episodic != nil {
			_ = memMgr.Episodic.Save(ctx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: "cron:failures",
				Role:      memory.RoleSystem,
				Content:   "[CRON FAILURE] " + summary,
				CreatedAt: time.Now(),
			})
		}
	})
	if err := cronDaemon.Start(); err != nil {
		log.Warn("failed to start cron daemon", zap.Error(err))
	} else {
		log.Info("cron daemon started", zap.Int("static_jobs", len(cronJobs)))
		defer cronDaemon.Stop()
	}

	// ─────────────────────────────────────────────────────────────

	if sessionID != "" {
		// Restore session history into working memory
		msgs, err := memMgr.Episodic.GetSession(ctx, sessionID, 200)
		if err == nil {
			wm := memMgr.GetWorking(sessionID)
			for _, m := range msgs {
				wm.Append(m)
			}
		}
	}

	// Autonomous mode
	if auto && message != "" {
		log.Info("Starting autonomous mode", zap.String("goal", message))
		fmt.Printf("\n🚀 Starting autonomous agent. Goal: %s\n\n", message)
		result, err := runner.RunAutonomous(ctx, message)
		if err != nil {
			return err
		}
		fmt.Printf("\n✅ Autonomous agent finished. Response: %s\n", result.Response)
		return nil
	}

	// Single message mode
	if message != "" {
		if noStream {
			result, err := runner.Run(ctx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: sessionID, // using the sessionID derived earlier or "cli"
				Role:      memory.RoleUser,
				Content:   message,
				CreatedAt: time.Now(),
			})
			if err != nil {
				return err
			}
			fmt.Println(result.Response)
			return nil
		}
		// Streaming mode
		done := make(chan error, 1)
		runner.RunStream(ctx, memory.Message{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Role:      memory.RoleUser,
			Content:   message,
			CreatedAt: time.Now(),
		}, &cliStreamHandler{done: done})
		return <-done
	}

	// Initialize Voice Client
	var voiceClient *voice.Client
	if cfg.Voice.Enabled {
		voiceClient = voice.NewClient(cfg.Voice)
	}

	// Start Messaging Channels
	msgHandler := channels.MessageHandler(runner.HandleChannelMessage)
	var emailSvc *email.Service
	var tgCh *telegram.Channel
	var dcCh *discord.Channel
	var slCh *slack.Channel
	var waCh *whatsapp.Channel
	activeChannels := 0
	reliableOutbound := map[string]*chadapter.ReliableSender{}
	reliabilityCfg := chadapter.ReliabilityConfig{
		Retry: chadapter.RetryPolicy{
			MaxAttempts:   cfg.Channels.Outbound.MaxAttempts,
			BaseDelay:     time.Duration(cfg.Channels.Outbound.BaseDelayMS) * time.Millisecond,
			MaxDelay:      time.Duration(cfg.Channels.Outbound.MaxDelayMS) * time.Millisecond,
			JitterPercent: cfg.Channels.Outbound.JitterPercent,
		},
		Breaker: chadapter.CircuitBreakerPolicy{
			FailureThreshold: cfg.Channels.Outbound.BreakerThreshold,
			Cooldown:         time.Duration(cfg.Channels.Outbound.BreakerCooldownS) * time.Second,
		},
		DLQPath: cfg.Channels.Outbound.DLQPath,
	}
	if cfg.Channels.Telegram != nil {
		requireMention := true
		if cfg.Channels.Telegram.RequireMention != nil {
			requireMention = *cfg.Channels.Telegram.RequireMention
		}
		tg, err := telegram.New(
			cfg.Channels.Telegram.BotToken,
			cfg.Channels.Telegram.DMMode,
			cfg.Channels.Telegram.AllowFrom,
			requireMention,
		)
		if err == nil {
			tgRS := chadapter.NewReliableSender("telegram", tg, reliabilityCfg)
			tg.WithReliableOutbound(tgRS)
			reliableOutbound["telegram"] = tgRS
			channelSenders["telegram"] = reliableToolSender{
				rs: tgRS,
			}
			tgCh = tg
			log.Info("Telegram channel active")
			activeChannels++
		}
	}
	if cfg.Channels.Discord != nil {
		discordRequireMention := true
		if cfg.Channels.Discord.RequireMention != nil {
			discordRequireMention = *cfg.Channels.Discord.RequireMention
		}
		dc, err := discord.New(
			cfg.Channels.Discord.BotToken,
			cfg.Channels.Discord.DMMode,
			cfg.Channels.Discord.AllowFrom,
			discordRequireMention,
			voiceClient,
		)
		if err == nil {
			dcRS := chadapter.NewReliableSender("discord", dc, reliabilityCfg)
			dc.WithReliableOutbound(dcRS)
			reliableOutbound["discord"] = dcRS
			channelSenders["discord"] = reliableToolSender{
				rs: dcRS,
			}
			dcCh = dc
			log.Info("Discord channel active")
			activeChannels++
		}
	}
	if cfg.Channels.Slack != nil {
		sl, err := slack.New(cfg.Channels.Slack.BotToken, cfg.Channels.Slack.AppToken, cfg.Channels.Slack.DMMode, cfg.Channels.Slack.AllowFrom)
		if err == nil {
			slRS := chadapter.NewReliableSender("slack", sl, reliabilityCfg)
			sl.WithReliableOutbound(slRS)
			reliableOutbound["slack"] = slRS
			channelSenders["slack"] = reliableToolSender{
				rs: slRS,
			}
			slCh = sl
			log.Info("Slack channel active")
			activeChannels++
		}
	}
	if cfg.Channels.WhatsApp != nil {
		wa, err := whatsapp.New(filepath.Join(cfg.StateDir, "whatsapp.db"), cfg.Channels.WhatsApp.SessionID, cfg.Channels.WhatsApp.DMMode, cfg.Channels.WhatsApp.AllowFrom, gf.logLevel, voiceClient)
		if err == nil {
			waCh = wa
			log.Info("WhatsApp channel active")
			activeChannels++
		}
	}

	if cfg.Email.Enabled {
		emailStore, err := email.OpenStore(cfg.StateDir)
		if err != nil {
			return fmt.Errorf("email store: %w", err)
		}
		defer emailStore.Close()
		emailModel := cfg.Email.Model
		if emailModel == "" {
			emailModel = modelInfo.ID
		}
		emailSvc, err = email.NewService(cfg, emailStore, p, emailModel, channelSenders, reliableOutbound, log)
		if err != nil {
			return fmt.Errorf("email service: %w", err)
		}
		if emailSvc != nil {
			msgHandler = emailSvc.WrapHandler(msgHandler)
			go emailSvc.Run(ctx)
			log.Info("email assistant started")
		}
	}

	if tgCh != nil {
		go tgCh.Start(ctx, msgHandler)
	}
	if dcCh != nil {
		go dcCh.Start(ctx, msgHandler)
	}
	if slCh != nil {
		go slCh.Start(ctx, msgHandler)
	}
	if waCh != nil {
		go waCh.Start(ctx, msgHandler)
	}

	// ── Proactive Engine ─────────────────────────────────────────────────
	proactiveEng := proactive.NewEngine(log)
	proactiveEng.RegisterTrigger(proactive.NewManualTrigger())

	// Register cron triggers from config so rules can subscribe to scheduled events.
	for _, cj := range cfg.Cron {
		proactiveEng.RegisterTrigger(proactive.NewCronTrigger("cron:"+cj.ID, cj.Schedule, map[string]any{"job_id": cj.ID}))
	}

	// Register email triggers for each configured account.
	if cfg.Email.Enabled {
		emailStoreTriggers, err := email.OpenStore(cfg.StateDir)
		if err != nil {
			log.Warn("proactive email store open failed, email triggers disabled", zap.Error(err))
		} else {
			for _, acc := range cfg.Email.Accounts {
				be, err := email.NewBackendForAccount(cfg, acc, emailStoreTriggers)
				if err != nil {
					log.Warn("proactive email trigger backend failed, skipping account",
						zap.String("account", acc.Name),
						zap.Error(err),
					)
					continue
				}
				proactiveEng.RegisterTrigger(
					proactive.NewCircuitBreaker(proactive.NewEmailTrigger(acc.Name, be, log), log),
				)
				log.Info("proactive email trigger registered (with circuit breaker)", zap.String("account", acc.Name))
			}
			defer emailStoreTriggers.Close()
		}
	}

	// Register calendar trigger if enabled.
	if cfg.Calendar.Enabled {
		calSrc, err := proactive.NewGoogleCalendarSource(ctx, cfg.StateDir, cfg.Calendar.TokenFile, cfg.Calendar.CalendarID)
		if err != nil {
			log.Warn("calendar source init failed, calendar trigger disabled", zap.Error(err))
		} else {
			pollInterval := 60 * time.Second
			if cfg.Calendar.PollInterval != "" {
				if d, err := time.ParseDuration(cfg.Calendar.PollInterval); err == nil {
					pollInterval = d
				}
			}
			warnBefore := 10 * time.Minute
			if cfg.Calendar.WarnBefore != "" {
				if d, err := time.ParseDuration(cfg.Calendar.WarnBefore); err == nil {
					warnBefore = d
				}
			}
			proactiveEng.RegisterTrigger(
				proactive.NewCircuitBreaker(
					proactive.NewCalendarTrigger("primary", calSrc, pollInterval, warnBefore, log), log,
				),
			)
			// Feed the awareness store so the agent always knows the next event.
			awareness.StartCalendarFeed(ctx, awareStore, func(c context.Context, from, to time.Time) ([]awareness.CalendarEvent, error) {
				evs, err := calSrc.ListUpcoming(c, from, to)
				if err != nil {
					return nil, err
				}
				out := make([]awareness.CalendarEvent, 0, len(evs))
				for _, e := range evs {
					out = append(out, awareness.CalendarEvent{ID: e.ID, Title: e.Title, StartTime: e.StartTime, Attendees: e.Attendees})
				}
				return out, nil
			}, 5*time.Minute)
			log.Info("calendar trigger registered",
				zap.String("calendar_id", cfg.Calendar.CalendarID),
				zap.Duration("poll", pollInterval),
				zap.Duration("warn_before", warnBefore),
			)
		}
	}

	// Register the agent runner as the default action.
	proactiveEng.RegisterAction(proactive.NewRunAgentAction(proactive.NewRunnerAdapter(runner)))

	// Register notifiers for active channels.
	// Channel notifiers are wrapped in Outbox for SQLite-backed retry persistence.
	outboxDBPath := filepath.Join(cfg.StateDir, "proactive-outbox.db")
	var outboxes []*proactive.Outbox
	registerOutbox := func(inner proactive.Notifier) {
		ob, err := proactive.NewOutbox(inner, outboxDBPath, log)
		if err != nil {
			log.Warn("failed to create outbox for notifier, using raw",
				zap.String("notifier", inner.Name()),
				zap.Error(err),
			)
			proactiveEng.RegisterNotifier(inner)
			return
		}
		ob.Start()
		outboxes = append(outboxes, ob)
		proactiveEng.RegisterNotifier(ob)
	}

	if tgCh != nil {
		tgSessionID := "tg:proactive"
		if strings.EqualFold(cfg.Email.Notify.Channel, "telegram") && cfg.Email.Notify.SessionID != "" {
			tgSessionID = cfg.Email.Notify.SessionID
		}
		registerOutbox(proactive.NewTelegramNotifier(tgCh, tgSessionID))
	}
	if dcCh != nil {
		dcSessionID := "discord:proactive:general"
		if strings.EqualFold(cfg.Email.Notify.Channel, "discord") && cfg.Email.Notify.SessionID != "" {
			dcSessionID = cfg.Email.Notify.SessionID
		}
		registerOutbox(proactive.NewDiscordNotifier(dcCh, dcSessionID))
	}
	if slCh != nil {
		registerOutbox(proactive.NewChannelNotifier("slack", func(ctx context.Context, sessionID, text string) error {
			_, err := slCh.Send(ctx, chadapter.OutboundMessage{SessionID: sessionID, Text: text})
			return err
		}))
	}
	// Console notifier does not need outbox wrapping — it can't fail in recoverable ways.
	proactiveEng.RegisterNotifier(proactive.NewWriterNotifier("console", os.Stdout))

	// Ensure outboxes are stopped and closed on shutdown.
	defer func() {
		for _, ob := range outboxes {
			ob.Close()
		}
	}()

	// Load initial rules from disk.
	rulesFilePath := filepath.Join(cfg.StateDir, "rules.yaml")
	if rules, err := proactiveEng.LoadRulesFromYAML(rulesFilePath); err == nil {
		if err := proactiveEng.SetRules(rules); err != nil {
			log.Warn("failed to set proactive rules", zap.Error(err))
		} else {
			log.Info("proactive rules loaded", zap.Int("count", len(rules)))
		}
	} else if !os.IsNotExist(err) {
		log.Warn("failed to load proactive rules", zap.Error(err))
	}

	proactiveEng.Start(ctx)
	defer proactiveEng.Stop()

	// Start the rule file watcher for hot-reload.
	ruleWatcher := proactive.NewRuleWatcher(rulesFilePath, proactiveEng, log)
	go func() {
		if err := ruleWatcher.Start(ctx); err != nil && err != context.Canceled {
			log.Warn("rule watcher exited", zap.Error(err))
		}
	}()

	// Send any pending crash reports from previous runs.
	if pending, err := crashreport.ScanPending(cfg.StateDir); err == nil && len(pending) > 0 {
		for _, path := range pending {
			data, err := os.ReadFile(path)
			if err != nil {
				log.Warn("failed to read crash report", zap.String("path", path), zap.Error(err))
				continue
			}
			// Try to notify via console; if telegram/discord are configured they
			// would need a notifier lookup. For now, log and move to sent.
			log.Warn("previous crash detected", zap.String("report", string(data)))
			if err := crashreport.MarkSent(path); err != nil {
				log.Warn("failed to mark crash report sent", zap.String("path", path), zap.Error(err))
			}
		}
	}

	// Heartbeats: periodic synthetic turns on a dedicated session (no chat spam).
	hb := cfg.Agent.Heartbeat
	if hb.Enabled && (serve || activeChannels > 0) {
		iv := hb.Interval
		if iv == "" {
			iv = "30m"
		}
		dur, err := time.ParseDuration(iv)
		if err != nil {
			log.Warn("heartbeat: invalid interval, using 30m", zap.String("interval", iv), zap.Error(err))
			dur = 30 * time.Minute
		}
		sid := hb.SessionID
		if sid == "" {
			sid = "assistclaw:heartbeat"
		}
		prompt := strings.TrimSpace(hb.Prompt)
		go runHeartbeatLoop(ctx, runner, dur, sid, prompt, log)
	}

	// If --serve is active, start the Gateway (+ embedded web UI) and wait
	if serve {
		pidFile := PidFile(cfg.StateDir)
		if oldPid, err := ReadPID(pidFile); err == nil && CheckPID(oldPid) {
			return fmt.Errorf("AssistClaw is already running (PID: %d). Stop it first with 'assistclaw stop'.", oldPid)
		}

		if err := WritePID(pidFile); err != nil {
			log.Warn("failed to write PID file", zap.Error(err))
		}
		defer os.Remove(pidFile)

		log.Info("Background mode active (v3 core engine)",
			zap.Bool("gateway", true),
			zap.Int("channels", activeChannels),
			zap.Int("pid", os.Getpid()),
		)

		srv := gateway.NewServer(cfg.Gateway.Port)
		srv.Bind = cfg.Gateway.Bind
		srv.Tailscale.Mode = cfg.Gateway.Tailscale.Mode
		srv.Token = cfg.Gateway.Token
		srv.Runner = runner
		srv.Version = version
		srv.Logger = log
		srv.MemMgr = memMgr
		srv.Provider = p
		srv.Proactive = proactiveEng
		srv.Awareness = awareStore

		// Determine public-facing address for the web UI
		webHost := cfg.Gateway.Host
		if webHost == "" {
			webHost = "localhost"
		}
		webURL := fmt.Sprintf("http://%s:%d", webHost, cfg.Gateway.Port)
		fmt.Printf("\n🌐 Web UI: %s\n", webURL)
		fmt.Printf("   Token: %s\n\n", cfg.Gateway.Token)

		go func() {
			if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
				log.Error("gateway failure", zap.Error(err))
			}
		}()

		// Wait for shutdown signal
		<-ctx.Done()
		log.Info("Shutting down background service...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer shutdownCancel()

		// 1. Stop accepting new channel messages.
		if tgCh != nil {
			_ = tgCh.Stop()
		}
		if dcCh != nil {
			_ = dcCh.Stop()
		}
		if slCh != nil {
			_ = slCh.Stop()
		}
		if waCh != nil {
			_ = waCh.Stop()
		}

		// 2. Stop cron daemon and proactive engine.
		if cronDaemon != nil {
			cronDaemon.Stop()
		}
		if proactiveEng != nil {
			proactiveEng.Stop()
		}

		// 3. Close outbox workers.
		for _, ob := range outboxes {
			_ = ob.Close()
		}

		// 4. Wait for in-flight agent runs (30s max).
		if !runner.WaitForInflight(30 * time.Second) {
			log.Warn("some agent runs did not finish within shutdown window")
		}

		// 5. Stop gateway.
		stopCtx, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()
		if err := srv.Stop(stopCtx); err != nil {
			log.Warn("gateway shutdown error", zap.Error(err))
		}
		return nil
	}

	// Interactive REPL mode
	return runREPL(ctx, runner, log, gf.noMouse)
}

// extractBundledSkills copies the repo's skills/ directory into destDir (bundled dir).
// It searches for the skills directory relative to the binary, CWD, or common install paths.
func extractBundledSkills(destDir string) error {
	// Check if already populated (skip to avoid overwriting user edits)
	if info, err := os.ReadDir(destDir); err == nil && len(info) > 0 {
		return nil // already extracted
	}

	// Find the source skills directory
	src := resolveBundledSkillsSrc()
	if src == "" {
		// Not available (e.g., installed without repo) — that's OK, marketplace will handle it
		return nil
	}

	return skills.CopyDir(src, destDir)
}

// resolveBundledSkillsSrc locates the bundled skills/ directory relative to common install paths.
func resolveBundledSkillsSrc() string {
	candidates := []string{}

	// 1. Relative to the running binary
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "skills"))
	}

	// 2. Relative to CWD (development mode)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "skills"))
	}

	// 3. Common install locations
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".assistclaw", "repo", "skills"))
	}
	candidates = append(candidates,
		"/usr/local/share/assistclaw/skills",
		"/opt/assistclaw/skills",
	)

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func buildLogger(level string) *zap.Logger {
	lvl := zap.WarnLevel
	switch strings.ToLower(level) {
	case "debug":
		lvl = zap.DebugLevel
	case "info":
		lvl = zap.InfoLevel
	case "warn":
		lvl = zap.WarnLevel
	case "error":
		lvl = zap.ErrorLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.EncoderConfig.TimeKey = "t"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	log, _ := cfg.Build()
	return log
}

func registerProviders(ctx context.Context, cfg *config.Config, reg *provider.Registry, log *zap.Logger) error {
	prov := cfg.Providers
	register := func(p provider.Provider) {
		if err := reg.Register(ctx, p); err != nil {
			log.Warn("provider registration warning", zap.String("provider", p.Name()), zap.Error(err))
		}
	}

	if prov.OpenAI != nil {
		register(openai.New(openai.Config{
			APIKey: prov.OpenAI.APIKey, BaseURL: prov.OpenAI.BaseURL,
			DefaultModel: prov.OpenAI.DefaultModel,
		}))
	}
	if prov.AzureOpenAI != nil {
		register(openai.New(openai.Config{
			APIKey: prov.AzureOpenAI.APIKey, BaseURL: prov.AzureOpenAI.BaseURL,
			IsAzure: true, APIVersion: prov.AzureOpenAI.APIVersion,
		}))
	}
	if prov.Anthropic != nil {
		register(anthropic.New(anthropic.Config{
			APIKey: prov.Anthropic.APIKey, BaseURL: prov.Anthropic.BaseURL,
			DefaultModel: prov.Anthropic.DefaultModel,
		}))
	}
	if prov.Bedrock != nil {
		p, err := bedrock.New(bedrock.Config{
			Region: prov.Bedrock.Region, Profile: prov.Bedrock.Profile,
			AccessKeyID: prov.Bedrock.AccessKeyID, SecretAccessKey: prov.Bedrock.SecretAccessKey,
			APIKey:       prov.Bedrock.APIKey,
			DefaultModel: prov.Bedrock.DefaultModel,
		})
		if err != nil {
			log.Warn("bedrock init failed", zap.Error(err))
		} else {
			register(p)
		}
	}
	if prov.Ollama != nil {
		register(ollama.New(ollama.Config{
			BaseURL: prov.Ollama.BaseURL, DefaultModel: prov.Ollama.DefaultModel,
		}))
	}
	if prov.VLLM != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "vllm", BaseURL: prov.VLLM.BaseURL, APIKey: prov.VLLM.APIKey,
			DefaultModel: prov.VLLM.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.LMStudio != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "lmstudio", BaseURL: prov.LMStudio.BaseURL,
			DefaultModel: prov.LMStudio.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.Groq != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "groq", BaseURL: "https://api.groq.com", APIKey: prov.Groq.APIKey,
			DefaultModel:   prov.Groq.DefaultModel,
			StaticModels:   catalogs.GroqModels("groq"),
			DiscoverModels: true,
		}))
	}
	if prov.Mistral != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "mistral", BaseURL: "https://api.mistral.ai", APIKey: prov.Mistral.APIKey,
			DefaultModel:   prov.Mistral.DefaultModel,
			StaticModels:   catalogs.MistralModels("mistral"),
			DiscoverModels: true,
		}))
	}
	if prov.Together != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "together", BaseURL: "https://api.together.xyz", APIKey: prov.Together.APIKey,
			DefaultModel:   prov.Together.DefaultModel,
			StaticModels:   catalogs.TogetherModels("together"),
			DiscoverModels: true,
		}))
	}
	if prov.OpenRouter != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "openrouter", BaseURL: "https://openrouter.ai/api", APIKey: prov.OpenRouter.APIKey,
			DefaultModel:   prov.OpenRouter.DefaultModel,
			StaticModels:   catalogs.OpenRouterModels("openrouter"),
			DiscoverModels: true,
			ExtraHeaders: map[string]string{
				"HTTP-Referer": prov.OpenRouter.SiteURL,
				"X-Title":      prov.OpenRouter.SiteName,
			},
		}))
	}
	if prov.NVIDIA != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "nvidia", BaseURL: "https://integrate.api.nvidia.com", APIKey: prov.NVIDIA.APIKey,
			DefaultModel:   prov.NVIDIA.DefaultModel,
			StaticModels:   catalogs.NVIDIAModels("nvidia"),
			DiscoverModels: true,
		}))
	}
	if prov.Cohere != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "cohere", BaseURL: "https://api.cohere.com", APIKey: prov.Cohere.APIKey,
			DefaultModel:   prov.Cohere.DefaultModel,
			StaticModels:   catalogs.CohereModels("cohere"),
			DiscoverModels: true,
		}))
	}
	if prov.HuggingFace != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "huggingface", BaseURL: prov.HuggingFace.BaseURL, APIKey: prov.HuggingFace.APIKey,
			DefaultModel: prov.HuggingFace.DefaultModel,
		}))
	}
	if prov.DeepSeek != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: prov.DeepSeek.APIKey,
			DefaultModel:   prov.DeepSeek.DefaultModel,
			StaticModels:   catalogs.DeepSeekModels("deepseek"),
			DiscoverModels: true,
		}))
	}
	if prov.Perplexity != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "perplexity", BaseURL: "https://api.perplexity.ai", APIKey: prov.Perplexity.APIKey,
			DefaultModel:   prov.Perplexity.DefaultModel,
			StaticModels:   catalogs.PerplexityModels("perplexity"),
			DiscoverModels: true,
		}))
	}
	if prov.XAI != nil {
		xaiModel := prov.XAI.DefaultModel
		if xaiModel == "" {
			xaiModel = "grok-4"
		}
		register(openaicompat.New(openaicompat.Config{
			Name:           "xai",
			BaseURL:        "https://api.x.ai/v1",
			APIKey:         prov.XAI.APIKey,
			DefaultModel:   xaiModel,
			StaticModels:   catalogs.XAIModels("xai"),
			DiscoverModels: true,
		}))
	}
	if prov.Vertex != nil {
		v, err := vertex.New(ctx, vertex.Config{
			ProjectID:    prov.Vertex.ProjectID,
			Location:     prov.Vertex.Location,
			Credentials:  prov.Vertex.Credentials,
			DefaultModel: prov.Vertex.DefaultModel,
		})
		if err != nil {
			log.Warn("vertex init failed", zap.Error(err))
		} else {
			register(v)
		}
	}
	// ─── Plano Smart Routing ───────────────────────────────────────────────────
	// If Plano is enabled, register it as the primary provider so all requests
	// flow through Plano's complexity-aware router. All other providers are still
	// registered so Plano can delegate to them, and as fallback if Plano is down.
	if cfg.Plano.Enabled {
		// Convert config.PlanoPreference → planoprovider.Preference
		prefs := make([]planoprovider.Preference, len(cfg.Plano.Preferences))
		for i, p := range cfg.Plano.Preferences {
			prefs[i] = planoprovider.Preference{
				Description: p.Description,
				PreferModel: p.PreferModel,
			}
		}

		// Look up fallback from already-registered providers
		var fallback provider.Provider
		if cfg.Plano.FallbackProvider != "" {
			if p, ok := reg.Get(cfg.Plano.FallbackProvider); ok {
				fallback = p
			}
		}

		planoP := planoprovider.New(planoprovider.Config{
			Enabled:          true,
			Endpoint:         cfg.Plano.Endpoint,
			FallbackProvider: cfg.Plano.FallbackProvider,
			Preferences:      prefs,
		}, fallback)

		register(planoP)
		log.Info("Plano smart routing enabled",
			zap.String("endpoint", cfg.Plano.Endpoint),
			zap.Int("preferences", len(prefs)),
		)
	}
	// ──────────────────────────────────────────────────────────────────────────

	return nil
}

// mergeBedrockForEmbeds fills in embeddings.bedrock from providers.bedrock when the embeddings
// block only sets default_model (or is partial). Otherwise LoadDefaultConfig falls through to
// EC2 IMDS and hangs or errors on laptops.
func mergeBedrockForEmbeds(ec *config.BedrockCreds, prov *config.BedrockCreds) *config.BedrockCreds {
	if ec == nil && prov == nil {
		return nil
	}
	var out config.BedrockCreds
	if ec != nil {
		out = *ec
	}
	if prov != nil {
		if out.Region == "" {
			out.Region = prov.Region
		}
		if out.Profile == "" {
			out.Profile = prov.Profile
		}
		if out.AccessKeyID == "" {
			out.AccessKeyID = prov.AccessKeyID
		}
		if out.SecretAccessKey == "" {
			out.SecretAccessKey = prov.SecretAccessKey
		}
		if out.APIKey == "" {
			out.APIKey = prov.APIKey
		}
		if out.DefaultModel == "" {
			out.DefaultModel = prov.DefaultModel
		}
	}
	if out.Region == "" {
		out.Region = "us-east-1"
	}
	return &out
}

func registerEmbedders(ctx context.Context, cfg *config.Config, reg *embeddings.Registry, log *zap.Logger) {
	ec := cfg.Embeddings
	register := func(e embeddings.Embedder) {
		if err := reg.Register(ctx, e); err != nil {
			log.Warn("embedder registration warning", zap.String("provider", e.Name()), zap.Error(err))
		}
	}

	// Register in priority order
	for _, name := range ec.Priority {
		switch name {
		case "openai":
			if ec.OpenAI != nil {
				register(embedproviders.NewOpenAI(ec.OpenAI.APIKey, ec.OpenAI.BaseURL))
			} else if cfg.Providers.OpenAI != nil {
				register(embedproviders.NewOpenAI(cfg.Providers.OpenAI.APIKey, ""))
			}
		case "azure":
			if ec.AzureOpenAI != nil {
				register(embedproviders.NewAzure(ec.AzureOpenAI.APIKey, ec.AzureOpenAI.BaseURL, ec.AzureOpenAI.APIVersion))
			} else if cfg.Providers.AzureOpenAI != nil {
				register(embedproviders.NewAzure(cfg.Providers.AzureOpenAI.APIKey, cfg.Providers.AzureOpenAI.BaseURL, cfg.Providers.AzureOpenAI.APIVersion))
			}
		case "ollama":
			if ec.OllamaEmbed != nil {
				register(embedproviders.NewOllama(ec.OllamaEmbed.BaseURL))
			} else if cfg.Providers.Ollama != nil {
				register(embedproviders.NewOllama(cfg.Providers.Ollama.BaseURL))
			} else {
				register(embedproviders.NewOllama(""))
			}
		case "bedrock":
			b := mergeBedrockForEmbeds(ec.Bedrock, cfg.Providers.Bedrock)
			if b != nil {
				e, err := embedproviders.NewBedrock(b.Region, b.Profile, b.AccessKeyID, b.SecretAccessKey, b.APIKey)
				if err != nil {
					log.Warn("bedrock embedder failed to initialize; semantic memory may be unavailable",
						zap.Error(err))
				} else {
					register(e)
				}
			}
		case "cohere":
			if ec.Cohere != nil {
				register(embedproviders.NewCohere(ec.Cohere.APIKey))
			} else if cfg.Providers.Cohere != nil {
				register(embedproviders.NewCohere(cfg.Providers.Cohere.APIKey))
			}
		case "google":
			if ec.Google != nil {
				register(embedproviders.NewGoogle(ec.Google.APIKey))
			}
		case "huggingface":
			if ec.HuggingFace != nil {
				register(embedproviders.NewHuggingFace(ec.HuggingFace.APIKey, ec.HuggingFace.BaseURL, ec.HuggingFace.Model))
			}
		case "voyage":
			if ec.Voyage != nil {
				register(embedproviders.NewVoyage(ec.Voyage.APIKey, ec.Voyage.BaseURL))
			} else if cfg.Providers.Voyage != nil {
				register(embedproviders.NewVoyage(cfg.Providers.Voyage.APIKey, cfg.Providers.Voyage.BaseURL))
			}
		case "mistral":
			if ec.Mistral != nil {
				register(embedproviders.NewMistral(ec.Mistral.APIKey, ec.Mistral.BaseURL))
			} else if cfg.Providers.Mistral != nil {
				register(embedproviders.NewMistral(cfg.Providers.Mistral.APIKey, ""))
			}
		case "vertex":
			v := ec.Vertex
			if v == nil && cfg.Providers.Vertex != nil {
				v = cfg.Providers.Vertex
			}
			if v != nil {
				e, err := embedproviders.NewVertex(ctx, v.ProjectID, v.Location, v.Credentials)
				if err == nil {
					register(e)
				}
			}
		}
	}
}

// cliStreamHandler prints tokens to stdout as they arrive.
type cliStreamHandler struct {
	done chan<- error
}

func (h *cliStreamHandler) OnToken(token string) { fmt.Print(token) }
func (h *cliStreamHandler) OnToolCall(name string, _ json.RawMessage) {
	fmt.Printf("\n[calling tool: %s]\n", name)
}
func (h *cliStreamHandler) OnToolResult(name, result string) {
	fmt.Printf("[%s result: %s]\n", name, truncate(result, 100))
}
func (h *cliStreamHandler) OnDone(result *agent.RunResult) {
	fmt.Printf("\n\n[%d iterations, %d tokens]\n", result.Iterations, result.Usage.TotalTokens)
	h.done <- nil
}
func (h *cliStreamHandler) OnError(err error) { h.done <- err }

// runREPL launches the interactive agent REPL.
// It now uses the futuristic bubbletea TUI from cmd/assistclaw/tui.
func runREPL(ctx context.Context, r *agent.Runner, log *zap.Logger, noMouse bool) error {
	// Count providers and skills for the banner
	providerCount := 1 // at least one is configured or we wouldn't be here
	skillCount := 0
	if home, err := os.UserHomeDir(); err == nil {
		if entries, err := os.ReadDir(filepath.Join(home, ".assistclaw", "skills", "custom")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					skillCount++
				}
			}
		}
	}

	// Wrap agent.Runner as tui.AgentRunner
	a := &agentRunnerAdapter{runner: r}
	return tui.RunREPL(ctx, a, version, providerCount, skillCount, noMouse)
}

// agentRunnerAdapter wraps agent.Runner to satisfy tui.AgentRunner.
type agentRunnerAdapter struct {
	runner *agent.Runner
}

func (a *agentRunnerAdapter) SessionID() string { return a.runner.SessionID() }

func (a *agentRunnerAdapter) RunStream(ctx context.Context, msg string, h tui.AgentStreamHandler) {
	a.runner.RunStream(ctx, memory.Message{
		ID:        uuid.New().String(),
		SessionID: a.runner.SessionID(),
		Role:      memory.RoleUser,
		Content:   msg,
		CreatedAt: time.Now(),
	}, &tuiStreamAdapter{h: h})
}

// tuiStreamAdapter translates agent.StreamHandler callbacks into the TUI's
// mirror types.
type tuiStreamAdapter struct {
	h tui.AgentStreamHandler
}

func (s *tuiStreamAdapter) OnToken(token string)                          { s.h.OnToken(token) }
func (s *tuiStreamAdapter) OnToolCall(name string, in json.RawMessage)    { s.h.OnToolCall(name, in) }
func (s *tuiStreamAdapter) OnToolResult(name, result string)              { s.h.OnToolResult(name, result) }
func (s *tuiStreamAdapter) OnError(err error)                             { s.h.OnError(err) }
func (s *tuiStreamAdapter) OnDone(res *agent.RunResult) {
	out := &tui.RunResult{}
	if res != nil {
		out.Iterations = res.Iterations
		out.Usage = struct{ TotalTokens int }{TotalTokens: res.Usage.TotalTokens}
	}
	s.h.OnDone(out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
