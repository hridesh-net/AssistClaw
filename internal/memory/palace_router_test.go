package memory

import "testing"

func TestInferTaxonomy(t *testing.T) {
	sourceType, palace, wing, room, tags := inferTaxonomy("memory/2026-04-14.md")
	if sourceType != "workspace_markdown" {
		t.Fatalf("unexpected source type: %s", sourceType)
	}
	if palace != "workspace" || wing != "memory" || room != "2026-04-14.md" {
		t.Fatalf("unexpected taxonomy: %s/%s/%s", palace, wing, room)
	}
	if len(tags) == 0 {
		t.Fatal("expected tags")
	}
}

func TestHeuristicPalaceRouter_Route(t *testing.T) {
	r := NewHeuristicPalaceRouter()
	route := r.Route("please update sprint docs and runbook")
	if route.Wing == "" {
		t.Fatal("expected wing classification for docs query")
	}
	if route.Confidence <= 0 {
		t.Fatal("expected positive confidence")
	}
}

func TestMatchesIncludeExclude(t *testing.T) {
	if !matchesIncludeExclude("memory/day.md", []string{"memory/*.md"}, nil) {
		t.Fatal("expected include to match memory markdown")
	}
	if matchesIncludeExclude("memory/archive/day.md", []string{"memory/*.md"}, []string{"memory/archive/*"}) {
		t.Fatal("expected exclude to take precedence")
	}
}
