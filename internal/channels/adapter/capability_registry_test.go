package adapter

import (
	"fmt"
	"sync"
	"testing"
)

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

func TestRegisterCapabilities_ConcurrentAccess(t *testing.T) {
	t.Helper()
	const workers = 16

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("testchannel-%d", i)
			RegisterCapabilities(name, ChannelCapabilities{
				Threading:        i%2 == 0,
				Attachments:      true,
				DirectMessages:   true,
				GroupMessages:    true,
				Mentions:         true,
				MaxMessageLength: 1000 + i,
			})
			got, ok := CapabilitiesFor(name)
			if !ok {
				t.Errorf("expected capability for %s", name)
				return
			}
			if got.MaxMessageLength != 1000+i {
				t.Errorf("unexpected maxMessageLength for %s: %d", name, got.MaxMessageLength)
			}
		}()
	}
	wg.Wait()
}
