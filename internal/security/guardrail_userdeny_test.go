package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func bashInput(cmd string) string {
	b, _ := json.Marshal(map[string]string{"command": cmd})
	return string(b)
}

func writeInput(path string) string {
	b, _ := json.Marshal(map[string]string{"path": path, "content": "x"})
	return string(b)
}

func TestUserDenyPaths_BlocksBashTouchingDeniedDir(t *testing.T) {
	g, err := NewGuardrail(ModeEnforce, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.WithUserDenyPaths([]string{"/etc", "/var/lib/postgresql/"})
	res := g.CheckToolCall("bash", bashInput("cat /etc/hostname"), "/state", "/work")
	if res.Action != ActionBlock {
		t.Fatalf("expected BLOCK, got %v findings=%v", res.Action, res.Findings)
	}
	if !strings.Contains(res.Message, "blocked") {
		t.Fatalf("message should mention block: %q", res.Message)
	}
}

func TestUserDenyPaths_HomeExpansion(t *testing.T) {
	old := osUserHomeDir
	osUserHomeDir = func() (string, error) { return "/home/me", nil }
	t.Cleanup(func() { osUserHomeDir = old })

	g, _ := NewGuardrail(ModeEnforce, nil, nil)
	g.WithUserDenyPaths([]string{"~/Documents/important"})

	res := g.CheckToolCall("write_file", writeInput("/home/me/Documents/important/notes.md"), "/state", "/work")
	if res.Action != ActionBlock {
		t.Fatalf("expected BLOCK for path under denied home dir, got %v", res.Action)
	}
}

func TestUserDenyPaths_TrailingSlashIsStrict(t *testing.T) {
	g, _ := NewGuardrail(ModeEnforce, nil, nil)
	g.WithUserDenyPaths([]string{"/etc/"})

	// Inside the dir → block
	if r := g.CheckToolCall("bash", bashInput("touch /etc/foo"), "/state", "/work"); r.Action != ActionBlock {
		t.Fatalf("expected BLOCK for /etc/foo, got %v", r.Action)
	}
	// Look-alike sibling /etcd → must NOT block
	if r := g.CheckToolCall("bash", bashInput("touch /etcd/data"), "/state", "/work"); r.Action == ActionBlock {
		t.Fatalf("expected ALLOW for /etcd/data under /etc/ rule, got %v", r.Action)
	}
}

func TestNewBashPatterns_BrewGitDocker(t *testing.T) {
	g, _ := NewGuardrail(ModeEnforce, nil, nil)
	cases := []string{
		"git reset --hard HEAD~1",
		"git push --force origin main",
		"docker rm -f $(docker ps -aq)",
		"npm uninstall -g typescript",
		"rm -rf $HOME/code",
		"sudo dd if=/dev/zero of=/dev/sda",
	}
	for _, cmd := range cases {
		r := g.CheckToolCall("bash", bashInput(cmd), "/state", "/work")
		if r.Action != ActionBlock {
			t.Errorf("expected BLOCK for %q, got %v findings=%v", cmd, r.Action, r.Findings)
		}
	}
}
