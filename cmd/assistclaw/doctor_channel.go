package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
)

func formatChannelTokenError(label string, err error) string {
	if err == nil {
		return ""
	}
	return sanitizeDoctorMessage(fmt.Sprintf("%s: %v", label, err))
}

// checkWebhooks documents gateway requirements for incoming webhooks (doctor does not start the HTTP server).
func checkWebhooks(cfg *config.Config) doctorCheck {
	if cfg == nil || !cfg.Webhooks.Enabled {
		return doctorCheck{
			ID:       "webhooks.gateway",
			Severity: "skipped",
			Message:  "webhooks.enabled is false (no incoming webhook routes).",
		}
	}
	if len(cfg.Webhooks.Mappings) == 0 {
		return doctorCheck{
			ID:       "webhooks.gateway",
			Severity: "skipped",
			Message:  "webhooks enabled but no mappings; add webhooks.mappings entries.",
		}
	}
	return doctorCheck{
		ID:       "webhooks.gateway",
		Severity: "skipped",
		Message:  "Incoming webhooks require a running gateway and a reachable public URL. Doctor does not start the server — after `assistclaw gateway start`, verify your ingress or curl the webhook path. See doc/runbooks/doctor-config-validation.md.",
	}
}

// Channel token format checks run before any outbound API call (one cheap check per channel).

var telegramBotTokenFormat = regexp.MustCompile(`^\d{6,}:[A-Za-z0-9_-]{25,}$`)

// validateTelegramBotTokenFormat returns a descriptive error if the token cannot be a valid BotFather token.
func validateTelegramBotTokenFormat(token string) error {
	t := strings.TrimSpace(token)
	if t == "" {
		return fmt.Errorf("bot_token is empty")
	}
	if !telegramBotTokenFormat.MatchString(t) {
		return fmt.Errorf("bot_token format invalid (expected <digits>:<secret> from @BotFather, no spaces)")
	}
	return nil
}

// validateDiscordBotTokenFormat checks the raw token (without "Bot " prefix).
func validateDiscordBotTokenFormat(token string) error {
	t := strings.TrimSpace(strings.TrimPrefix(token, "Bot "))
	if t == "" {
		return fmt.Errorf("bot_token is empty")
	}
	parts := strings.Split(t, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("bot_token format invalid (expected three dot-separated segments from Discord Developer Portal)")
	}
	return nil
}

// validateSlackTokenFormats checks Bot User OAuth Token and App-level token prefixes.
func validateSlackTokenFormats(botToken, appToken string) error {
	b := strings.TrimSpace(botToken)
	a := strings.TrimSpace(appToken)
	if b == "" || a == "" {
		return fmt.Errorf("bot_token and app_token are required")
	}
	if !strings.HasPrefix(b, "xoxb-") {
		return fmt.Errorf("bot_token should start with xoxb- (Bot User OAuth Token from Slack app OAuth & Permissions)")
	}
	if len(b) < 20 {
		return fmt.Errorf("bot_token looks too short")
	}
	if !strings.HasPrefix(a, "xapp-") {
		return fmt.Errorf("app_token should start with xapp- (App-level token with connections:write for Socket Mode)")
	}
	if len(a) < 20 {
		return fmt.Errorf("app_token looks too short")
	}
	return nil
}

// formatChannelPingError turns API/adapter errors into sanitized, actionable messages (never echoes tokens).
func formatChannelPingError(channelID, step string, err error) string {
	if err == nil {
		return ""
	}
	msg := sanitizeDoctorMessage(err.Error())
	if msg == "" {
		msg = "unknown error"
	}
	lower := strings.ToLower(msg)
	hint := ""
	switch channelID {
	case "channel.telegram":
		switch {
		case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
			hint = " — invalid or revoked bot token; get a new token from @BotFather"
		case strings.Contains(lower, "404"):
			hint = " — verify bot token matches the bot you expect"
		}
	case "channel.discord":
		switch {
		case strings.Contains(lower, "401"):
			hint = " — invalid bot token; reset in Discord Developer Portal → Bot"
		case strings.Contains(lower, "403"):
			hint = " — missing gateway intent or scope; enable Message Content Intent (and others your app needs) in the Developer Portal"
		case strings.Contains(lower, "429"):
			hint = " — rate limited; retry later"
		}
	case "channel.slack":
		switch {
		case strings.Contains(lower, "401") || strings.Contains(lower, "invalid_auth"):
			hint = " — invalid bot or app token; reinstall app to workspace or rotate tokens"
		case strings.Contains(lower, "403"):
			hint = " — missing OAuth scopes; ensure chat:write, app_mentions:read, and Socket Mode scopes as needed"
		}
	}
	if strings.Contains(lower, "403") && hint == "" {
		hint = " — forbidden; check token scopes and app install"
	}
	return fmt.Sprintf("%s: %s%s", step, msg, hint)
}
