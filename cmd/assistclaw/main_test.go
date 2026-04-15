package main

import (
	"strings"
	"testing"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/mcp"
	"github.com/assistclaw/assistclaw/internal/mempalace"
	"github.com/assistclaw/assistclaw/internal/skills"
)

func TestEffectiveMCPClients_autoStartAddsSyntheticStdio(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		StateDir: "/state",
		Memory: config.MemoryConfig{
			MemPalace: config.MemoryMemPalaceConfig{
				AutoStart:        true,
				MCPClientName:    "mempalace",
				PythonExecutable: "/venv/bin/python",
			},
		},
	}
	out, injected := effectiveMCPClients(cfg)
	if !injected {
		t.Fatal("expected synthetic MemPalace client to be injected")
	}
	if len(out) != 1 {
		t.Fatalf("len(out)=%d want 1", len(out))
	}
	c := out[0]
	if c.Name != "mempalace" || c.Transport != "stdio" || c.Command != "/venv/bin/python" {
		t.Fatalf("unexpected client: %+v", c)
	}
	if got, want := strings.Join(c.Args, " "), "-m mempalace.mcp_server"; got != want {
		t.Fatalf("args: got %q want %q", got, want)
	}
	if c.Dir != "" {
		t.Fatalf("Dir should be empty without managed_venv, got %q", c.Dir)
	}
}

func TestEffectiveMCPClients_managedVenvSetsWorldDir(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		StateDir: "/data/ac",
		Memory: config.MemoryConfig{
			MemPalace: config.MemoryMemPalaceConfig{
				AutoStart:        true,
				ManagedVenv:      true,
				PythonExecutable: "/data/ac/mempalace/venv/bin/python3",
			},
		},
	}
	out, injected := effectiveMCPClients(cfg)
	if !injected {
		t.Fatal("expected injection")
	}
	wantDir := mempalace.ManagedWorldDir(cfg.StateDir)
	if out[0].Dir != wantDir {
		t.Fatalf("Dir: got %q want %q", out[0].Dir, wantDir)
	}
}

func TestEffectiveMCPClients_skipWhenNameCollides(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		StateDir: "/state",
		MCP: config.MCPConfig{
			Clients: []config.MCPClientConfig{
				{Name: "mempalace", Transport: "stdio", Command: "python3", Args: []string{"-m", "mempalace.mcp_server"}},
			},
		},
		Memory: config.MemoryConfig{
			MemPalace: config.MemoryMemPalaceConfig{
				AutoStart:     true,
				MCPClientName: "mempalace",
			},
		},
	}
	out, injected := effectiveMCPClients(cfg)
	if injected {
		t.Fatal("should not inject when mcp.clients already defines mempalace")
	}
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
}

func TestEffectiveMCPClients_customClientName(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		StateDir: "/state",
		Memory: config.MemoryConfig{
			MemPalace: config.MemoryMemPalaceConfig{
				AutoStart:        true,
				MCPClientName:    "mp2",
				PythonExecutable: "python3",
			},
		},
	}
	out, injected := effectiveMCPClients(cfg)
	if !injected || len(out) != 1 || out[0].Name != "mp2" {
		t.Fatalf("injected=%v out=%+v", injected, out)
	}
}

func TestAugmentActiveSkillsWithMCP_appendsVirtualServers(t *testing.T) {
	t.Parallel()
	reg := skills.NewRegistry()
	reg.Register(&skills.Skill{Name: "mcp:demo", Nodes: map[string]*skills.Node{"SKILL": {Name: "SKILL"}}})
	got := augmentActiveSkillsWithMCP(reg, []string{"legal"})
	if len(got) != 2 || got[0] != "legal" || got[1] != "mcp:demo" {
		t.Fatalf("got %#v", got)
	}
}

func TestAugmentActiveSkillsWithMCP_dedupes(t *testing.T) {
	t.Parallel()
	reg := skills.NewRegistry()
	reg.Register(&skills.Skill{Name: "mcp:dup", Nodes: map[string]*skills.Node{}})
	got := augmentActiveSkillsWithMCP(reg, []string{"mcp:dup", "other"})
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestMCPClientConfigsFromYAML_passesDirAndEnv(t *testing.T) {
	t.Parallel()
	in := []config.MCPClientConfig{
		{
			Name: "mempalace", Transport: "stdio", Command: "/py", Args: []string{"-m", "mempalace.mcp_server"},
			Dir: "/world", Env: []string{"FOO=bar"},
		},
	}
	out := mcpClientConfigsFromYAML(in)
	if len(out) != 1 {
		t.Fatal(len(out))
	}
	c := out[0]
	if c.Name != "mempalace" || c.Dir != "/world" || len(c.Env) != 1 || c.Env[0] != "FOO=bar" {
		t.Fatalf("%+v", c)
	}
	if c.Transport != mcp.TransportStdio {
		t.Fatalf("transport %v", c.Transport)
	}
}
