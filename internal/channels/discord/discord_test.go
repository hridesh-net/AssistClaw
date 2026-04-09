package discord

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestSplitDiscordMessage(t *testing.T) {
	in := "hello " + strings.Repeat("x", 2205)
	out := splitDiscordMessage(in)
	if len(out) < 2 {
		t.Fatalf("expected split into multiple chunks, got %d", len(out))
	}
	for i, part := range out {
		if len(part) > 2000 {
			t.Fatalf("chunk %d exceeds discord limit: %d", i, len(part))
		}
	}
}

func TestParseDiscordSession(t *testing.T) {
	guildID, channelID, err := parseDiscordSession("discord:123:456")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if guildID != "123" || channelID != "456" {
		t.Fatalf("unexpected parse result: guild=%q channel=%q", guildID, channelID)
	}
}

func TestShouldAcceptMessageRequireMention(t *testing.T) {
	c := &Channel{
		requireMention: true,
		session: &discordgo.Session{
			State: &discordgo.State{
				Ready: discordgo.Ready{
					User: &discordgo.User{ID: "bot-1"},
				},
			},
		},
	}
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			GuildID: "guild-1",
			Content: "hello there",
			Author:  &discordgo.User{ID: "user-1"},
		},
	}
	if c.shouldAcceptMessage(msg) {
		t.Fatalf("expected unmentioned guild message to be blocked")
	}
	msg.Content = "hello <@bot-1>"
	if !c.shouldAcceptMessage(msg) {
		t.Fatalf("expected mentioned message to be accepted")
	}
}
