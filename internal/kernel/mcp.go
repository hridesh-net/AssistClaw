package kernel

import (
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/mcp"
	"github.com/assistclaw/assistclaw/internal/mempalace"
	"github.com/assistclaw/assistclaw/internal/skills"
)

// EffectiveMCPClients returns cfg.MCP.Clients plus an optional synthetic MemPalace stdio client
// when memory.mempalace.auto_start is true and no client with the same name is already defined.
func EffectiveMCPClients(cfg *config.Config) ([]config.MCPClientConfig, bool) {
	out := append([]config.MCPClientConfig(nil), cfg.MCP.Clients...)
	name := strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)
	if name == "" {
		name = "mempalace"
	}
	if !cfg.Memory.MemPalace.AutoStart {
		return out, false
	}
	for _, c := range out {
		if c.Name == name {
			return out, false
		}
	}
	py := strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable)
	if py == "" {
		py = "python3"
	}
	syn := config.MCPClientConfig{
		Name:      name,
		Transport: "stdio",
		Command:   py,
		Args:      []string{"-m", "mempalace.mcp_server"},
	}
	if cfg.Memory.MemPalace.ManagedVenv {
		syn.Dir = mempalace.ManagedWorldDir(cfg.StateDir)
	}
	out = append(out, syn)
	return out, true
}

// AugmentActiveSkillsWithMCP appends skill names registered by external MCP (prefix "mcp:")
// so they appear in the session skills header and skill_graph_index without requiring users
// to list every server in agent.enabled_skills.
func AugmentActiveSkillsWithMCP(skillReg skills.Registry, active []string) []string {
	seen := make(map[string]struct{}, len(active))
	for _, n := range active {
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
	}
	out := append([]string(nil), active...)
	for _, s := range skillReg.List() {
		name := s.Name
		if !strings.HasPrefix(name, "mcp:") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// MCPClientConfigsFromYAML converts config MCP client entries into mcp.ClientConfig values.
func MCPClientConfigsFromYAML(in []config.MCPClientConfig) []mcp.ClientConfig {
	out := make([]mcp.ClientConfig, 0, len(in))
	for _, c := range in {
		tr := mcp.TransportStdio
		if strings.EqualFold(strings.TrimSpace(c.Transport), "http") {
			tr = mcp.TransportHTTP
		}
		out = append(out, mcp.ClientConfig{
			Name:      c.Name,
			Transport: tr,
			Command:   c.Command,
			Args:      c.Args,
			Dir:       c.Dir,
			Env:       c.Env,
			URL:       c.URL,
			AuthToken: c.AuthToken,
		})
	}
	return out
}
