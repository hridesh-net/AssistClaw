package adapter

import "testing"

func TestCapabilitiesFor_Builtins(t *testing.T) {
	t.Helper()
	channels := []string{"telegram", "discord", "slack", "whatsapp"}
	for _, name := range channels {
		c, ok := CapabilitiesFor(name)
		if !ok {
			t.Fatalf("expected built-in capability for %s", name)
		}
		if c.MaxMessageLength <= 0 {
			t.Fatalf("invalid maxMessageLength for %s: %d", name, c.MaxMessageLength)
		}
		if !c.DirectMessages {
			t.Fatalf("expected DM support for %s", name)
		}
	}
}

func TestRegisterCapabilities_Extensible(t *testing.T) {
	t.Helper()
	RegisterCapabilities("msteams", ChannelCapabilities{
		Threading:        true,
		Attachments:      true,
		DirectMessages:   false,
		GroupMessages:    true,
		Mentions:         true,
		Voice:            false,
		Reactions:        true,
		Edits:            true,
		MaxMessageLength: 28000,
	})
	c, ok := CapabilitiesFor("msteams")
	if !ok {
		t.Fatal("expected registered channel capabilities")
	}
	if c.MaxMessageLength != 28000 || !c.Threading {
		t.Fatalf("unexpected capabilities: %+v", c)
	}
}
