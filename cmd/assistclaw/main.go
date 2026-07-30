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
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	chadapter "github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/awareness"
	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/channels/discord"
	"github.com/assistclaw/assistclaw/internal/channels/slack"
	"github.com/assistclaw/assistclaw/internal/channels/telegram"
	"github.com/assistclaw/assistclaw/internal/channels/whatsapp"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/crashreport"
	"github.com/assistclaw/assistclaw/internal/email"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/assistclaw/assistclaw/internal/gateway"
	"github.com/assistclaw/assistclaw/internal/kernel"
	"github.com/assistclaw/assistclaw/internal/mcp"
	"github.com/assistclaw/assistclaw/internal/memory"
	obstracing "github.com/assistclaw/assistclaw/internal/observability/tracing"
	"github.com/assistclaw/assistclaw/internal/proactive"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/skills"
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
// gateway command
// ─────────────────────────────────────────────

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// effectiveMCPClients returns cfg.MCP.Clients plus an optional synthetic MemPalace stdio client
// when memory.mempalace.auto_start is true and no client with the same name is already defined.
func effectiveMCPClients(cfg *config.Config) ([]config.MCPClientConfig, bool) {
	return kernel.EffectiveMCPClients(cfg)
}

// augmentActiveSkillsWithMCP appends skill names registered by external MCP (prefix "mcp:")
// so they appear in the session skills header and skill_graph_index without requiring users
// to list every server in agent.enabled_skills.
func augmentActiveSkillsWithMCP(skillReg skills.Registry, active []string) []string {
	return kernel.AugmentActiveSkillsWithMCP(skillReg, active)
}

func mcpClientConfigsFromYAML(in []config.MCPClientConfig) []mcp.ClientConfig {
	return kernel.MCPClientConfigsFromYAML(in)
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

	app, err := kernel.Build(ctx, cfg, log, kernel.BuildOptions{
		Model:                   model,
		AllowSensitiveSkills:    gf.allowSensitiveSkills,
		AllowAllSensitiveSkills: gf.allowAllSensitiveSkills,
	})
	if err != nil {
		return err
	}
	defer app.Close()

	// Local aliases so the mode-dispatch and serve code below reads unchanged.
	memMgr := app.Mem
	runner := app.Runner
	p := app.Provider
	channelSenders := app.ChannelSenders
	awareStore := app.Aware
	cronDaemon := app.Cron

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
			emailModel = app.ModelID
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
	return kernel.ExtractBundledSkills(destDir)
}

// resolveBundledSkillsSrc locates the bundled skills/ directory relative to common install paths.
func resolveBundledSkillsSrc() string {
	return kernel.ResolveBundledSkillsSrc()
}

// buildLogger delegates to kernel.BuildLogger. Retained as a package-local
// wrapper so the many existing call sites across cmd/ compile unchanged.
func buildLogger(level string) *zap.Logger { return kernel.BuildLogger(level) }

// registerProviders delegates to kernel.RegisterProviders (see internal/kernel).
func registerProviders(ctx context.Context, cfg *config.Config, reg *provider.Registry, log *zap.Logger) error {
	return kernel.RegisterProviders(ctx, cfg, reg, log)
}

// registerEmbedders delegates to kernel.RegisterEmbedders (see internal/kernel).
func registerEmbedders(ctx context.Context, cfg *config.Config, reg *embeddings.Registry, log *zap.Logger) {
	kernel.RegisterEmbedders(ctx, cfg, reg, log)
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

func (s *tuiStreamAdapter) OnToken(token string)                       { s.h.OnToken(token) }
func (s *tuiStreamAdapter) OnToolCall(name string, in json.RawMessage) { s.h.OnToolCall(name, in) }
func (s *tuiStreamAdapter) OnToolResult(name, result string)           { s.h.OnToolResult(name, result) }
func (s *tuiStreamAdapter) OnError(err error)                          { s.h.OnError(err) }
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
