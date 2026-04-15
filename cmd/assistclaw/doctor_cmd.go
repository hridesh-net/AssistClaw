package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/internal/channels/discord"
	"github.com/assistclaw/assistclaw/internal/channels/slack"
	"github.com/assistclaw/assistclaw/internal/channels/telegram"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/localintel"
)

// doctorCheck is one row of assistclaw doctor output (text or JSON).
type doctorCheck struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // ok | warn | error | skipped
	Message  string `json:"message"`
}

type doctorOutput struct {
	SchemaVersion int           `json:"schema_version"`
	Checks        []doctorCheck `json:"checks"`
}

func doctorCmd(flags *globalFlags) *cobra.Command {
	var asJSON, skipNetwork bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate config and channel connectivity (read-only API checks)",
		Long: `Runs non-destructive checks:

  • Loads configuration (no interactive onboarding — uses config file if present, else env-only defaults).
  • For Telegram, Discord, and Slack: Bot API / auth.test via adapter Ping (credentials + network).
  • For WhatsApp: reports whether the local session database exists (legacy channel; no cloud API ping).

Use --json for machine-readable output. Use --skip-network to skip channel API calls (e.g. air-gapped).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			cfg, err := loadConfigForDoctor(flags.configPath)
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			checks := runDoctorChecks(ctx, cfg, skipNetwork)

			if asJSON {
				out := doctorOutput{SchemaVersion: 1, Checks: checks}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return err
				}
			} else {
				for _, c := range checks {
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n  %s\n", c.Severity, c.ID, c.Message)
				}
			}

			for _, c := range checks {
				if c.Severity == "error" {
					return fmt.Errorf("doctor: one or more checks failed")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON (schema_version + checks)")
	cmd.Flags().BoolVar(&skipNetwork, "skip-network", false, "Skip Telegram/Discord/Slack API pings")
	return cmd
}

// loadConfigForDoctor loads YAML when the file exists; otherwise env-only defaults (no onboarding wizard).
func loadConfigForDoctor(configPath string) (*config.Config, error) {
	path := configPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return config.Load(path)
	}
	if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	return config.LoadFromEnv(), nil
}

func runDoctorChecks(ctx context.Context, cfg *config.Config, skipNetwork bool) []doctorCheck {
	var checks []doctorCheck

	if skipNetwork {
		checks = append(checks, doctorCheck{
			ID:       "channels.network",
			Severity: "skipped",
			Message:  "Channel API checks skipped (--skip-network).",
		})
	} else {
		checks = append(checks, pingTelegram(ctx, cfg)...)
		checks = append(checks, pingDiscord(ctx, cfg)...)
		checks = append(checks, pingSlack(ctx, cfg)...)
	}

	checks = append(checks, checkWhatsAppSession(cfg))
	checks = append(checks, checkLocalIntel(cfg))
	return checks
}

func checkLocalIntel(cfg *config.Config) doctorCheck {
	if !cfg.Agent.LocalIntel.Enabled {
		return doctorCheck{
			ID:       "agent.local_intel",
			Severity: "skipped",
			Message:  "agent.local_intel.enabled is false.",
		}
	}
	var warnings []string
	if !localintel.CompiledWithLocalGemma() {
		warnings = append(warnings, "this binary does not include in-process Gemma support (use official release v3.10.15+ or build with tag assistclaw_localgemma)")
	}
	p := strings.TrimSpace(cfg.Agent.LocalIntel.GGUFPath)
	if p == "" {
		p = strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF"))
	}
	if p != "" {
		if _, err := os.Stat(p); err != nil {
			warnings = append(warnings, fmt.Sprintf("GGUF not readable at %q: %v", p, err))
		}
	} else if len(localintel.Gemma4E2BGGUF) == 0 {
		warnings = append(warnings, "run `assistclaw local-intel setup` (or set agent.local_intel.gguf_path / ASSISTCLAW_LOCAL_GEMMA_GGUF), or build with assistclaw_embedlocalgemma and ship embedded weights")
	}
	if len(warnings) > 0 {
		return doctorCheck{
			ID:       "agent.local_intel",
			Severity: "warn",
			Message:  strings.Join(warnings, " | "),
		}
	}
	return doctorCheck{
		ID:       "agent.local_intel",
		Severity: "ok",
		Message:  "local_intel is enabled; GGUF/embed path looks consistent with this binary.",
	}
}

func pingTelegram(ctx context.Context, cfg *config.Config) []doctorCheck {
	ch := cfg.Channels.Telegram
	if ch == nil || ch.BotToken == "" {
		return []doctorCheck{{
			ID:       "channel.telegram",
			Severity: "skipped",
			Message:  "Not configured (no channels.telegram or bot_token).",
		}}
	}
	requireMention := true
	if ch.RequireMention != nil {
		requireMention = *ch.RequireMention
	}
	tg, err := telegram.New(ch.BotToken, ch.DMMode, ch.AllowFrom, requireMention)
	if err != nil {
		return []doctorCheck{{
			ID:       "channel.telegram",
			Severity: "error",
			Message:  fmt.Sprintf("init: %v", err),
		}}
	}
	if err := tg.Ping(ctx); err != nil {
		return []doctorCheck{{
			ID:       "channel.telegram",
			Severity: "error",
			Message:  fmt.Sprintf("getMe: %v", err),
		}}
	}
	return []doctorCheck{{
		ID:       "channel.telegram",
		Severity: "ok",
		Message:  "Telegram Bot API reachable (getMe).",
	}}
}

func pingDiscord(ctx context.Context, cfg *config.Config) []doctorCheck {
	ch := cfg.Channels.Discord
	if ch == nil || ch.BotToken == "" {
		return []doctorCheck{{
			ID:       "channel.discord",
			Severity: "skipped",
			Message:  "Not configured (no channels.discord or bot_token).",
		}}
	}
	requireMention := true
	if ch.RequireMention != nil {
		requireMention = *ch.RequireMention
	}
	dc, err := discord.New(ch.BotToken, ch.DMMode, ch.AllowFrom, requireMention, nil)
	if err != nil {
		return []doctorCheck{{
			ID:       "channel.discord",
			Severity: "error",
			Message:  fmt.Sprintf("init: %v", err),
		}}
	}
	if err := dc.Ping(ctx); err != nil {
		return []doctorCheck{{
			ID:       "channel.discord",
			Severity: "error",
			Message:  fmt.Sprintf("@me: %v", err),
		}}
	}
	return []doctorCheck{{
		ID:       "channel.discord",
		Severity: "ok",
		Message:  "Discord API reachable (GET /users/@me).",
	}}
}

func pingSlack(ctx context.Context, cfg *config.Config) []doctorCheck {
	ch := cfg.Channels.Slack
	if ch == nil || ch.BotToken == "" || ch.AppToken == "" {
		return []doctorCheck{{
			ID:       "channel.slack",
			Severity: "skipped",
			Message:  "Not configured (need channels.slack bot_token + app_token).",
		}}
	}
	sl, err := slack.New(ch.BotToken, ch.AppToken, ch.DMMode, ch.AllowFrom)
	if err != nil {
		return []doctorCheck{{
			ID:       "channel.slack",
			Severity: "error",
			Message:  fmt.Sprintf("init: %v", err),
		}}
	}
	if err := sl.Ping(ctx); err != nil {
		return []doctorCheck{{
			ID:       "channel.slack",
			Severity: "error",
			Message:  fmt.Sprintf("auth.test: %v", err),
		}}
	}
	return []doctorCheck{{
		ID:       "channel.slack",
		Severity: "ok",
		Message:  "Slack API reachable (auth.test).",
	}}
}

func checkWhatsAppSession(cfg *config.Config) doctorCheck {
	wa := cfg.Channels.WhatsApp
	if wa == nil {
		return doctorCheck{
			ID:       "channel.whatsapp",
			Severity: "skipped",
			Message:  "Not configured (legacy channel; no adapter Ping).",
		}
	}
	dbPath := filepath.Join(cfg.StateDir, "whatsapp.db")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return doctorCheck{
				ID:       "channel.whatsapp",
				Severity: "warn",
				Message:  fmt.Sprintf("Session DB not found at %s — link with onboard/start before use.", dbPath),
			}
		}
		return doctorCheck{
			ID:       "channel.whatsapp",
			Severity: "error",
			Message:  fmt.Sprintf("stat %s: %v", dbPath, err),
		}
	}
	return doctorCheck{
		ID:       "channel.whatsapp",
		Severity: "ok",
		Message:  fmt.Sprintf("Session store present (%s). Linking state is local-only; run daemon to verify.", dbPath),
	}
}
