package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSensitiveSkillRefusesWithoutApprover(t *testing.T) {
	skill := &Skill{
		Name:      "secrets",
		Sensitive: true,
		Tools: []SkillTool{
			{Name: "fetch", Description: "Read a secret", Command: "/bin/true"},
		},
	}
	tools := ConvertTools(skill, "/tmp", nil) // nil approver = default-deny
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	_, err := tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when running sensitive skill without approval")
	}
	if !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("error should mention sensitivity: %v", err)
	}
}

func TestSensitiveSkillRunsWhenApproved(t *testing.T) {
	skill := &Skill{
		Name:      "secrets",
		Sensitive: true,
		Tools:     []SkillTool{{Name: "noop", Description: "ok", Command: "/bin/echo hi"}},
	}
	tools := ConvertTools(skill, "/tmp", func(s, _ string) bool { return s == "secrets" })
	out, err := tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("expected echo output, got %q", out)
	}
}

func TestNonSensitiveSkillIgnoresApprover(t *testing.T) {
	skill := &Skill{
		Name:      "plain",
		Sensitive: false,
		Tools:     []SkillTool{{Name: "echo", Description: "ok", Command: "/bin/echo plain"}},
	}
	tools := ConvertTools(skill, "/tmp", func(string, string) bool { return false })
	out, err := tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("plain skill should run regardless of approver: %v", err)
	}
	if !strings.Contains(out, "plain") {
		t.Fatalf("expected echo output, got %q", out)
	}
}
