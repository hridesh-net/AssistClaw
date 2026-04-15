package memory

// This file implements lightweight AssistClaw-local taxonomy routing only.
// The MemPalace project (https://github.com/MemPalace/mempalace) is a separate Python system;
// use MCP (python -m mempalace.mcp_server) plus memory.mempalace in assistclaw.yaml to attach the real MemPalace server.

import "strings"

// PalaceRoute captures taxonomy hints for retrieval routing.
type PalaceRoute struct {
	Palace     string
	Wing       string
	Room       string
	Tags       []string
	Confidence float64
}

// MatchesDocument returns true when the route aligns with a document taxonomy.
func (r PalaceRoute) MatchesDocument(d Document) bool {
	if strings.TrimSpace(r.Palace) != "" && !strings.EqualFold(r.Palace, d.Palace) {
		return false
	}
	if strings.TrimSpace(r.Wing) != "" && !strings.EqualFold(r.Wing, d.Wing) {
		return false
	}
	if strings.TrimSpace(r.Room) != "" && !strings.EqualFold(r.Room, d.Room) {
		return false
	}
	return true
}

// IsZero reports whether the route has no practical constraints.
func (r PalaceRoute) IsZero() bool {
	return strings.TrimSpace(r.Palace) == "" &&
		strings.TrimSpace(r.Wing) == "" &&
		strings.TrimSpace(r.Room) == "" &&
		len(r.Tags) == 0
}

// PalaceRouter decides taxonomy hints from user queries.
type PalaceRouter interface {
	Route(query string) PalaceRoute
}

// HeuristicPalaceRouter maps broad intents to stable taxonomy segments for AssistClaw’s sqlite-vec metadata.
// It is unrelated to the MemPalace Python project.
type HeuristicPalaceRouter struct{}

func NewHeuristicPalaceRouter() *HeuristicPalaceRouter {
	return &HeuristicPalaceRouter{}
}

func (r *HeuristicPalaceRouter) Route(query string) PalaceRoute {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return PalaceRoute{}
	}
	switch {
	case containsAny(q, "sprint", "story", "runbook", "doc", "documentation"):
		return PalaceRoute{Palace: "workspace", Wing: "doc", Confidence: 0.84, Tags: []string{"docs"}}
	case containsAny(q, "test", "benchmark", "load", "regression"):
		return PalaceRoute{Palace: "workspace", Wing: "internal", Room: "memory", Confidence: 0.78, Tags: []string{"tests"}}
	case containsAny(q, "memory", "retrieval", "semantic", "episodic", "chunk"):
		return PalaceRoute{Palace: "workspace", Wing: "memory", Confidence: 0.81, Tags: []string{"memory"}}
	case containsAny(q, "config", "yaml", "setting", "flag"):
		return PalaceRoute{Palace: "workspace", Wing: "config", Confidence: 0.71, Tags: []string{"ops"}}
	default:
		return PalaceRoute{Palace: "workspace", Confidence: 0.45}
	}
}

func containsAny(s string, candidates ...string) bool {
	for _, c := range candidates {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}
