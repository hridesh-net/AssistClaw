package main

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateTelegramBotTokenFormat(t *testing.T) {
	t.Parallel()
	if err := validateTelegramBotTokenFormat("123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd"); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if err := validateTelegramBotTokenFormat("bad"); err == nil {
		t.Fatal("expected error for bad token")
	}
}

func TestValidateDiscordBotTokenFormat(t *testing.T) {
	t.Parallel()
	good := "x." + strings.Repeat("a", 6) + "." + strings.Repeat("b", 27)
	if err := validateDiscordBotTokenFormat(good); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if err := validateDiscordBotTokenFormat("no-dots"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateSlackTokenFormats(t *testing.T) {
	t.Parallel()
	if err := validateSlackTokenFormats("xoxb-"+strings.Repeat("x", 30), "xapp-"+strings.Repeat("y", 30)); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if err := validateSlackTokenFormats("bad", "xapp-yyy"); err == nil {
		t.Fatal("expected error for bad bot token")
	}
}

func TestFormatChannelPingError_Telegram401(t *testing.T) {
	t.Parallel()
	err := errors.New(`telegram getMe failed: Unauthorized (401)`)
	m := formatChannelPingError("channel.telegram", "getMe", err)
	if !strings.Contains(m, "401") {
		t.Fatalf("expected status context: %q", m)
	}
	if !strings.Contains(m, "BotFather") && !strings.Contains(m, "token") {
		t.Fatalf("expected hint: %q", m)
	}
}

func TestFormatChannelPingError_Discord403(t *testing.T) {
	t.Parallel()
	err := errors.New(`discord @me failed: 403 Forbidden`)
	m := formatChannelPingError("channel.discord", "GET /users/@me", err)
	if !strings.Contains(m, "Intent") && !strings.Contains(m, "Portal") {
		// hint may say "intent" lowercase
		if !strings.Contains(strings.ToLower(m), "intent") {
			t.Fatalf("expected scope hint: %q", m)
		}
	}
}
