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

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/autotool"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	embedproviders "github.com/assistclaw/assistclaw/internal/embeddings/providers"
	"github.com/assistclaw/assistclaw/internal/gateway"
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

	// Channels
	"github.com/assistclaw/assistclaw/internal/channels/discord"
	"github.com/assistclaw/assistclaw/internal/channels/slack"
	"github.com/assistclaw/assistclaw/internal/channels/telegram"
	"github.com/assistclaw/assistclaw/internal/channels/whatsapp"
)

const version = "v1.0.28-debug"

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
		providersCmd(flags),
		embeddingsCmd(flags),
		memoryCmd(flags),
		toolsCmd(flags),
		gatewayCmd(flags),
		onboardCmd(flags),
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
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck

			cfg, err := loadConfig(gf.configPath, log)
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

			// Load Skills
			skillReg := skills.NewRegistry()
			if err := skillReg.LoadAll(ctx, cfg.Agent.SkillsDir); err != nil {
				log.Warn("failed to load skills", zap.Error(err))
			}
			var activeSkillNames []string
			for _, s := range skillReg.List() {
				activeSkillNames = append(activeSkillNames, s.Name)
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
			for _, t := range tools.Default(memSearchFn, memSnippetFn) {
				if tool, ok := t.(agent.Tool); ok {
					toolReg.Register(tool)
				}
			}

			runner := agent.NewRunner(agent.Config{
				MaxIterations:       cfg.Agent.MaxIterations,
				Model:               modelInfo.ID,
				ActiveSkillsContext: skillsCtx,
			}, p, toolReg, memMgr, log, cfg.StateDir)

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
				tg, err := telegram.New(cfg.Channels.Telegram.BotToken)
				if err == nil {
					go tg.Start(ctx, runner.HandleChannelMessage)
					log.Info("Telegram channel active")
					activeChannels++
				}
			}
			if cfg.Channels.Discord != nil {
				dc, err := discord.New(cfg.Channels.Discord.BotToken)
				if err == nil {
					go dc.Start(ctx, runner.HandleChannelMessage)
					log.Info("Discord channel active")
					activeChannels++
				}
			}
			if cfg.Channels.Slack != nil {
				sl, err := slack.New(cfg.Channels.Slack.BotToken, cfg.Channels.Slack.AppToken)
				if err == nil {
					go sl.Start(ctx, runner.HandleChannelMessage)
					log.Info("Slack channel active")
					activeChannels++
				}
			}
			if cfg.Channels.WhatsApp != nil {
				wa, err := whatsapp.New(filepath.Join(cfg.StateDir, "whatsapp.db"), cfg.Channels.WhatsApp.SessionID)
				if err == nil {
					go wa.Start(ctx, runner.HandleChannelMessage)
					log.Info("WhatsApp channel active")
					activeChannels++
				}
			}

			// If --serve is active, start the Gateway too and wait
			if serve {
				log.Info("Background mode active (v3 core engine)",
					zap.Bool("gateway", true),
					zap.Int("channels", activeChannels),
				)

				srv := gateway.NewServer(cfg.Gateway.Port)
				srv.Bind = cfg.Gateway.Bind
				srv.Tailscale.Mode = cfg.Gateway.Tailscale.Mode

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
	cmd := &cobra.Command{Use: "tools", Short: "Manage auto-generated tools"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all persisted auto-generated tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			creator, err := autotool.NewCreator(autotool.CreatorConfig{
				ToolsDir: cfg.Agent.ToolsDir,
				VenvPath: filepath.Join(cfg.StateDir, "venv"),
				Timeout:  30,
			}, log)
			if err != nil {
				return err
			}
			toolList, err := creator.List()
			if err != nil {
				return err
			}
			if len(toolList) == 0 {
				fmt.Println("No auto-generated tools yet.")
				return nil
			}
			for _, t := range toolList {
				fmt.Printf("• %s — %s\n  Created: %s\n  Script: %s\n\n",
					t.Name, t.Description,
					t.CreatedAt.Format("2006-01-02 15:04"),
					t.ScriptPath,
				)
			}
			return nil
		},
	})
	return cmd
}

// ─────────────────────────────────────────────
// gateway command
// ─────────────────────────────────────────────

func gatewayCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "gateway",
		Short: "Start the AssistClaw REST/WebSocket Gateway server",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync()

			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			log.Info("Starting AssistClaw Gateway...",
				zap.String("host", cfg.Gateway.Host),
				zap.Int("port", cfg.Gateway.Port),
				zap.String("bind", cfg.Gateway.Bind),
			)
			srv := gateway.NewServer(cfg.Gateway.Port)
			srv.Bind = cfg.Gateway.Bind
			srv.Tailscale.Mode = cfg.Gateway.Tailscale.Mode

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
	}
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
		if err := runOnboarding(path); err != nil {
			log.Warn("interactive onboarding failed or was skipped, falling back to environment variables", zap.Error(err))
			return config.LoadFromEnv(), nil
		}
	}
	return config.Load(path)
}

func buildLogger(level string) *zap.Logger {
	lvl := zap.InfoLevel
	switch strings.ToLower(level) {
	case "debug":
		lvl = zap.DebugLevel
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

func runREPL(ctx context.Context, r *agent.Runner, log *zap.Logger) error {
	fmt.Printf("AssistClaw %s  (session: %s)\nType your message, or 'exit' to quit.\n\n", version, r.SessionID())
	done := make(chan error, 1)
	for {
		fmt.Print("You: ")
		var line string
		_, err := fmt.Scanln(&line)
		if err != nil || strings.TrimSpace(line) == "exit" {
			return nil
		}
		r.RunStream(ctx, line, &cliStreamHandler{done: done})
		if err := <-done; err != nil {
			log.Error("agent error", zap.Error(err))
		}
		fmt.Println()
	}
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
