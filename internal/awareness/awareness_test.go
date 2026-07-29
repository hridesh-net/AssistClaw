package awareness

import (
	"strings"
	"testing"
	"time"
)

func TestStoreSetGetTTL(t *testing.T) {
	s := NewStore("")
	s.Set("user.location", "home", 0)
	if sig, ok := s.Get("user.location"); !ok || sig.Value != "home" {
		t.Fatalf("get: %v %v", sig, ok)
	}

	s.Set("device.battery", "12%", time.Nanosecond)
	time.Sleep(time.Millisecond)
	if _, ok := s.Get("device.battery"); ok {
		t.Fatal("expired signal should be invisible")
	}
	if _, ok := s.Snapshot()["device.battery"]; ok {
		t.Fatal("expired signal should not appear in snapshot")
	}

	s.Delete("user.location")
	if _, ok := s.Get("user.location"); ok {
		t.Fatal("deleted signal should be gone")
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)
	s1.Set("user.location", "office", 0)

	s2 := NewStore(dir)
	if sig, ok := s2.Get("user.location"); !ok || sig.Value != "office" {
		t.Fatalf("snapshot did not survive restart: %v %v", sig, ok)
	}
}

func TestDaypartCoversAllHours(t *testing.T) {
	for h := 0; h < 24; h++ {
		ts := time.Date(2026, 6, 11, h, 0, 0, 0, time.Local)
		if Daypart(ts) == "" {
			t.Errorf("hour %d has no daypart", h)
		}
	}
	if got := Daypart(time.Date(2026, 6, 11, 14, 0, 0, 0, time.Local)); !strings.Contains(got, "afternoon") {
		t.Errorf("14:00 = %q", got)
	}
	if got := Daypart(time.Date(2026, 6, 11, 3, 0, 0, 0, time.Local)); !strings.Contains(got, "night") {
		t.Errorf("03:00 = %q", got)
	}
}

func TestPromptBlock(t *testing.T) {
	s := NewStore("")
	s.Set("calendar.next_event", `"Standup" at 10:00 (in 25m)`, 0)
	s.Set("user.activity", "active at the computer", 0)
	s.Set("custom.signal", "anything", 0)

	blk := s.PromptBlock()
	for _, want := range []string{
		"## Live Context",
		"Local time:",
		"Next calendar event",
		"User presence: active at the computer",
		"custom.signal: anything",
	} {
		if !strings.Contains(blk, want) {
			t.Errorf("prompt block missing %q:\n%s", want, blk)
		}
	}
}
