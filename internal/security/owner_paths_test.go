package security

import "testing"

func TestAbsMatchesOwnerOnly(t *testing.T) {
	state := "/home/x/.assistclaw"
	rules := []string{"POLICIES.md", "RULES.md", "policies"}

	tests := []struct {
		abs  string
		want bool
	}{
		{"/home/x/.assistclaw/POLICIES.md", true},
		{"/home/x/.assistclaw/RULES.md", true},
		{"/home/x/.assistclaw/policies/foo.yaml", true},
		{"/home/x/.assistclaw/policies", true},
		{"/home/x/.assistclaw/AGENTS.md", false},
		{"/home/x/.assistclaw/subagents/id/POLICIES.md", false},
		{"/tmp/POLICIES.md", false},
	}
	for _, tc := range tests {
		if got := AbsMatchesOwnerOnly(state, tc.abs, rules); got != tc.want {
			t.Errorf("AbsMatchesOwnerOnly(%q, %q) = %v, want %v", state, tc.abs, got, tc.want)
		}
	}
}

func TestPathsTouchedByToolWriteFile(t *testing.T) {
	state := "/state"
	ws := "/state"
	json := `{"path":"POLICIES.md","content":"x"}`
	paths := PathsTouchedByTool("write_file", json, state, ws)
	if len(paths) == 0 {
		t.Fatal("expected paths")
	}
	found := false
	for _, p := range paths {
		if p == "/state/POLICIES.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("got %v", paths)
	}
}

func TestBashTouchesOwnerAbsPaths(t *testing.T) {
	state := "/home/u/.assistclaw"
	rules := []string{"POLICIES.md", "policies"}
	cmd := "echo hi > /home/u/.assistclaw/POLICIES.md"
	paths := bashTouchesOwnerAbsPaths(cmd, state, rules)
	if len(paths) != 1 || paths[0] != "/home/u/.assistclaw/POLICIES.md" {
		t.Fatalf("got %v", paths)
	}
}
