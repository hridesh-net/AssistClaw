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
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/autotool"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	embedproviders "github.com/assistclaw/assistclaw/internal/embeddings/providers"
	"github.com/assistclaw/assistclaw/internal/gateway"
	"github.com/assistclaw/assistclaw/internal/graph"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/provider/anthropic"
	"github.com/assistclaw/assistclaw/internal/provider/bedrock"
	"github.com/assistclaw/assistclaw/internal/provider/ollama"
	"github.com/assistclaw/assistclaw/internal/provider/openai"
	"github.com/assistclaw/assistclaw/internal/provider/openaicompat"
	"github.com/assistclaw/assistclaw/internal/provider/vertex"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/assistclaw/assistclaw/internal/tools"
	_ "github.com/assistclaw/assistclaw/internal/webui" // ensure embed FS is included

	// Channels
	"github.com/assistclaw/assistclaw/internal/channels/discord"
	"github.com/assistclaw/assistclaw/internal/channels/slack"
	"github.com/assistclaw/assistclaw/internal/channels/telegram"
	"github.com/assistclaw/assistclaw/internal/channels/whatsapp"
	planoprovider "github.com/assistclaw/assistclaw/internal/provider/plano"
)

const version = "v3.4.2"

func main() {
	fmt.Fprintf(os.Stderr, "[assistclaw] version %s startup\n", version)
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// Global flags
// ─────────────────────────────────────────────

type globalFlags struct {
	configPath string
	logLevel   string
	noColor    bool
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

	root.AddCommand(
		agentCmd(flags),
		startCmd(flags),
		stopCmd(flags),
		statusCmd(flags),
		restartCmd(flags),
		providersCmd(flags),
		embeddingsCmd(flags),
		memoryCmd(flags),
		toolsCmd(flags),
		gatewayCmd(flags),
		onboardCmd(flags),
		skillsCmd(flags),
		mcpCmd(flags),
		serviceCmd(flags),
		versionCmd(),
	)
	return root
}

// ─────────────────────────────────────────────
// agent command
// ─────────────────────────────────────────────

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
			return runAgent(gf, gf.configPath, model, message, sessionID, serve, noStream)
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
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start AssistClaw in background mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			if daemon {
				return Detach("start")
			}
			return runAgent(gf, gf.configPath, "", "", "", true, false)
		},
	}
	cmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run detached in the background")
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
			})
		},
	}
}

func restartCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the background AssistClaw process",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = stopCmd(gf).RunE(cmd, args)
			time.Sleep(1 * time.Second)
			return startCmd(gf).RunE(cmd, args)
		},
	}
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

			prim := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
			dim := lipgloss.NewStyle().Foreground(tui.ColorMuted)
			header := lipgloss.NewStyle().Foreground(tui.ColorNeon).Bold(true)

			// ── Section 1: Built-in tools ──────────────────────────────────────
			fmt.Println(header.Render("\n⚡ Built-in System Tools") + dim.Render("  (always available)"))
			fmt.Println(dim.Render("─────────────────────────────────────────────────────────────"))
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
				fmt.Fprintf(w, "  %s\t%s\n", prim.Render(t.name), dim.Render(t.desc))
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

			fmt.Println(header.Render("\n🧠 Skill Tools") + dim.Render(fmt.Sprintf("  (%d installed skills)", len(allSkills))))
			fmt.Println(dim.Render("─────────────────────────────────────────────────────────────"))
			if skillToolCount == 0 {
				fmt.Println(dim.Render("  No skill tools installed yet."))
				fmt.Println(dim.Render("  Run: assistclaw skills install <name>"))
			} else {
				w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, s := range allSkills {
					for _, t := range s.Tools {
						fmt.Fprintf(w2, "  %s\t%s\t%s\n",
							prim.Render(t.Name),
							dim.Render("["+s.Name+"]"),
							dim.Render(t.Description))
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

			fmt.Println(header.Render("\n🔧 Auto-generated Tools") + dim.Render(fmt.Sprintf("  (%d generated)", len(autoList))))
			fmt.Println(dim.Render("─────────────────────────────────────────────────────────────"))
			if len(autoList) == 0 {
				fmt.Println(dim.Render("  No auto-generated tools yet."))
				fmt.Println(dim.Render("  Ask the agent to create one — it uses 'bash' and 'write_file' automatically."))
			} else {
				w3 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for _, t := range autoList {
					fmt.Fprintf(w3, "  %s\t%s\t%s\n",
						prim.Render(t.Name),
						dim.Render(t.CreatedAt.Format("2006-01-02")),
						dim.Render(t.Description))
				}
				w3.Flush()
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
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Manage the AssistClaw Gateway and Web UI",
		Long: `Manage the AssistClaw background gateway and embedded web UI.

Subcommands:
  start    Start in background daemon mode (web UI + channels)
  stop     Stop the running background daemon
  restart  Restart the background daemon
  serve    Run the gateway in the foreground (blocks terminal)
  status   Show daemon status (alias of 'assistclaw status')`,
	}

	// gateway start — alias of 'assistclaw start --daemon'
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start AssistClaw daemon in background (web UI + agent + channels)",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			webHost := cfg.Gateway.Host
			if webHost == "" {
				webHost = "localhost"
			}
			fmt.Printf("\n🌐 Web UI: http://%s:%d\n", webHost, cfg.Gateway.Port)

			errCh := make(chan error, 1)
			go func() {
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
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

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("assistclaw %s\n", version)
		},
	}
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func loadConfig(path string, log *zap.Logger) (*config.Config, error) {
	if path == "" {
		path = config.DefaultConfigPath()
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := runOnboarding(path); err != nil {
			log.Warn("interactive onboarding failed or was skipped, falling back to environment variables", zap.Error(err))
			return config.LoadFromEnv(), nil
		}
	}
	return config.Load(path)
}

func runAgent(gf *globalFlags, configPath string, model string, message string, sessionID string, serve bool, noStream bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := buildLogger(gf.logLevel)
	defer log.Sync() //nolint:errcheck

	cfg, err := loadConfig(configPath, log)
	if err != nil {
		return err
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

	p, modelInfo, err := reg.ResolveModel(resolvedModel)
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
	skillsCtx := skillReg.BuildContext(activeSkillNames)

	// Build tool registry
	toolReg := agent.NewToolRegistry()

	// Register tools from skills
	for _, s := range skillReg.List() {
		skillTools := skills.ConvertTools(&s, cfg.Agent.SkillsDir) // Simplification: assuming all tools relative to skills dir
		for _, t := range skillTools {
			toolReg.Register(t)
		}
	}

	memSearchFn := func(searchCtx context.Context, query string, limit int) ([]string, error) {
		var out []string

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
				for _, d := range docs {
					out = append(out, fmt.Sprintf("[semantic] [score:%.2f] [%s / %s] source=%s: %s", d.Score, d.Model, d.CreatedAt.Format("2006-01-02 15:04"), d.Source, d.Content))
				}
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
	for _, t := range tools.Default(memSearchFn, memSnippetFn, memMgr.Episodic, p, modelInfo.ID, nil) {
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

	runner := agent.NewRunner(agent.Config{
		MaxIterations:       cfg.Agent.MaxIterations,
		Model:               modelInfo.ID,
		ActiveSkillsContext: skillsCtx,
		ProviderName:        providerNameForCaps,
	}, p, toolReg, memMgr, log, cfg.StateDir).WithCatalog(catalog)

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

	// Single message mode
	if message != "" {
		if noStream {
			result, err := runner.Run(ctx, message)
			if err != nil {
				return err
			}
			fmt.Println(result.Response)
			return nil
		}
		// Streaming mode
		done := make(chan error, 1)
		runner.RunStream(ctx, message, &cliStreamHandler{done: done})
		return <-done
	}

	// Start Messaging Channels
	activeChannels := 0
	if cfg.Channels.Telegram != nil {
		tg, err := telegram.New(cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.DMMode, cfg.Channels.Telegram.AllowFrom)
		if err == nil {
			go tg.Start(ctx, runner.HandleChannelMessage)
			log.Info("Telegram channel active")
			activeChannels++
		}
	}
	if cfg.Channels.Discord != nil {
		dc, err := discord.New(cfg.Channels.Discord.BotToken, cfg.Channels.Discord.DMMode, cfg.Channels.Discord.AllowFrom)
		if err == nil {
			go dc.Start(ctx, runner.HandleChannelMessage)
			log.Info("Discord channel active")
			activeChannels++
		}
	}
	if cfg.Channels.Slack != nil {
		sl, err := slack.New(cfg.Channels.Slack.BotToken, cfg.Channels.Slack.AppToken, cfg.Channels.Slack.DMMode, cfg.Channels.Slack.AllowFrom)
		if err == nil {
			go sl.Start(ctx, runner.HandleChannelMessage)
			log.Info("Slack channel active")
			activeChannels++
		}
	}
	if cfg.Channels.WhatsApp != nil {
		wa, err := whatsapp.New(filepath.Join(cfg.StateDir, "whatsapp.db"), cfg.Channels.WhatsApp.SessionID, cfg.Channels.WhatsApp.DMMode, cfg.Channels.WhatsApp.AllowFrom, gf.logLevel)
		if err == nil {
			go wa.Start(ctx, runner.HandleChannelMessage)
			log.Info("WhatsApp channel active")
			activeChannels++
		}
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

		// Determine public-facing address for the web UI
		webHost := cfg.Gateway.Host
		if webHost == "" {
			webHost = "localhost"
		}
		webURL := fmt.Sprintf("http://%s:%d", webHost, cfg.Gateway.Port)
		fmt.Printf("\n🌐 Web UI: %s\n", webURL)
		fmt.Printf("   Token: %s\n\n", cfg.Gateway.Token)

		go func() {
			if err := srv.Start(); err != nil && err != http.ErrServerClosed {
				log.Error("gateway failure", zap.Error(err))
			}
		}()

		// Wait for shutdown signal
		<-ctx.Done()
		log.Info("Shutting down background service...")

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Stop(stopCtx); err != nil {
			log.Warn("gateway shutdown error", zap.Error(err))
		}
		return nil
	}

	// Interactive REPL mode
	return runREPL(ctx, runner, log)
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
			DefaultModel: prov.Groq.DefaultModel,
		}))
	}
	if prov.Mistral != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "mistral", BaseURL: "https://api.mistral.ai", APIKey: prov.Mistral.APIKey,
			DefaultModel: prov.Mistral.DefaultModel,
		}))
	}
	if prov.Together != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "together", BaseURL: "https://api.together.xyz", APIKey: prov.Together.APIKey,
			DefaultModel: prov.Together.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.OpenRouter != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "openrouter", BaseURL: "https://openrouter.ai/api", APIKey: prov.OpenRouter.APIKey,
			DefaultModel: prov.OpenRouter.DefaultModel, DiscoverModels: true,
			ExtraHeaders: map[string]string{
				"HTTP-Referer": prov.OpenRouter.SiteURL,
				"X-Title":      prov.OpenRouter.SiteName,
			},
		}))
	}
	if prov.NVIDIA != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "nvidia", BaseURL: "https://integrate.api.nvidia.com", APIKey: prov.NVIDIA.APIKey,
			DefaultModel: prov.NVIDIA.DefaultModel,
		}))
	}
	if prov.Cohere != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "cohere", BaseURL: "https://api.cohere.com", APIKey: prov.Cohere.APIKey,
			DefaultModel: prov.Cohere.DefaultModel,
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
			DefaultModel: prov.DeepSeek.DefaultModel, DiscoverModels: true,
		}))
	}
	if prov.Perplexity != nil {
		register(openaicompat.New(openaicompat.Config{
			Name: "perplexity", BaseURL: "https://api.perplexity.ai", APIKey: prov.Perplexity.APIKey,
			DefaultModel: prov.Perplexity.DefaultModel,
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
			b := ec.Bedrock
			if b == nil && cfg.Providers.Bedrock != nil {
				b = cfg.Providers.Bedrock
			}
			if b != nil {
				e, err := embedproviders.NewBedrock(b.Region, b.Profile, b.AccessKeyID, b.SecretAccessKey, b.APIKey)
				if err == nil {
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
func runREPL(ctx context.Context, r *agent.Runner, log *zap.Logger) error {
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
	return tui.RunREPL(ctx, a, version, providerCount, skillCount)
}

// agentRunnerAdapter wraps agent.Runner to satisfy tui.AgentRunner.
type agentRunnerAdapter struct {
	runner *agent.Runner
}

func (a *agentRunnerAdapter) SessionID() string { return a.runner.SessionID() }
func (a *agentRunnerAdapter) Run(ctx context.Context, msg string) (*tui.RunResult, error) {
	res, err := a.runner.Run(ctx, msg)
	if err != nil || res == nil {
		return nil, err
	}
	return &tui.RunResult{
		Iterations: res.Iterations,
		Usage:      struct{ TotalTokens int }{TotalTokens: res.Usage.TotalTokens},
	}, nil
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
