package proactive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRulesFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")

	data := `
rules:
  - id: cron-daily
    trigger: cron
    action: run_agent
    prompt: "Daily briefing"
    notify_to: [console]
    cooldown: 5m
    enabled: true
  - id: email-important
    trigger: email
    match:
      importance: "high"
    action: run_agent
    notify_to: [console]
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write test yaml: %v", err)
	}

	triggers := map[string]Trigger{"cron": NewCronTrigger("cron", "@daily", nil), "email": NewManualTrigger()}
	actions := map[string]Action{"run_agent": NewRunAgentAction(&fakeInvoker{})}
	notifiers := map[string]Notifier{"console": NewWriterNotifier("console", &noopWriter{})}

	rules, err := LoadRulesFromYAML(path, triggers, actions, notifiers)
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].ID != "cron-daily" {
		t.Fatalf("expected first rule id cron-daily, got %s", rules[0].ID)
	}
	if rules[0].Cooldown == 0 {
		t.Fatal("expected cooldown to be parsed")
	}
	if !rules[0].Enabled {
		t.Fatal("expected first rule to be enabled")
	}
	if rules[1].Match == nil {
		t.Fatal("expected second rule to have a match predicate")
	}
}

func TestLoadRulesFromYAML_UnknownTrigger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	os.WriteFile(path, []byte("rules:\n  - id: bad\n    trigger: unknown\n"), 0644)

	_, err := LoadRulesFromYAML(path, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown trigger")
	}
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (n int, err error) { return len(p), nil }
