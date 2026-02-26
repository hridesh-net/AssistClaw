package skills_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assistclaw/assistclaw/internal/skills"
)

// helper to create a skill directory with given files
func makeSkillDir(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, name)
	_ = os.MkdirAll(dir, 0o755)
	for fname, content := range files {
		_ = os.WriteFile(filepath.Join(dir, fname), []byte(content), 0o644)
	}
	return base // return parent (skills root)
}

// ─────────────────────────────────────────────
// 1. Basic node indexing
// ─────────────────────────────────────────────

func TestLoadAll_IndexesNodes(t *testing.T) {
	root := makeSkillDir(t, "cooking", map[string]string{
		"SKILL.md": `---
name: cooking
description: A skill for culinary tasks
---
This is the main cooking skill.
`,
		"knife-skills.md": `---
summary: Basic knife handling and safety
---
Always keep your fingers curled.
`,
		"sauces.md": `# Classic Sauces

How to make a roux, béchamel, and velouté.
`,
	})

	reg := skills.NewRegistry()
	if err := reg.LoadAll(context.Background(), root); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	skill, ok := reg.Get("cooking")
	if !ok {
		t.Fatal("expected skill 'cooking' to be loaded")
	}

	expectedNodes := []string{"SKILL", "knife-skills", "sauces"}
	for _, n := range expectedNodes {
		if _, ok := skill.Nodes[n]; !ok {
			t.Errorf("expected node %q to be indexed", n)
		}
	}
}

// ─────────────────────────────────────────────
// 2. Summary derivation: frontmatter > heading > first line
// ─────────────────────────────────────────────

func TestNodeSummary_FrontmatterTakesPriority(t *testing.T) {
	root := makeSkillDir(t, "sk", map[string]string{
		"SKILL.md": "---\nname: sk\n---\nBody.",
		"alpha.md": "---\nsummary: Explicit summary\n---\n# Different Heading\nBody.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	sk, _ := reg.Get("sk")
	node := sk.Nodes["alpha"]
	if node.Summary != "Explicit summary" {
		t.Errorf("expected 'Explicit summary', got %q", node.Summary)
	}
}

func TestNodeSummary_HeadingFallback(t *testing.T) {
	root := makeSkillDir(t, "sk2", map[string]string{
		"SKILL.md": "---\nname: sk2\n---\n",
		"beta.md":  "# Introduction to Risk Management\n\nDetails here.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	sk, _ := reg.Get("sk2")
	node := sk.Nodes["beta"]
	if node.Summary != "Introduction to Risk Management" {
		t.Errorf("expected heading as summary, got %q", node.Summary)
	}
}

func TestNodeSummary_FirstLineFallback(t *testing.T) {
	root := makeSkillDir(t, "sk3", map[string]string{
		"SKILL.md": "---\nname: sk3\n---\n",
		"gamma.md": "This is a plain note without headings.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	sk, _ := reg.Get("sk3")
	node := sk.Nodes["gamma"]
	if node.Summary != "This is a plain note without headings." {
		t.Errorf("expected first line as summary, got %q", node.Summary)
	}
}

func TestSKILLNodeGetsDescriptionAsSummary(t *testing.T) {
	root := makeSkillDir(t, "chef", map[string]string{
		"SKILL.md": "---\nname: chef\ndescription: Culinary assistant for meal planning\n---\nMain instructions.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	sk, _ := reg.Get("chef")
	skillNode := sk.Nodes["SKILL"]
	if skillNode.Summary != "Culinary assistant for meal planning" {
		t.Errorf("expected skill description as SKILL node summary, got %q", skillNode.Summary)
	}
}

// ─────────────────────────────────────────────
// 3. ReadSkillNode
// ─────────────────────────────────────────────

func TestReadSkillNode_ReturnsFullBody(t *testing.T) {
	root := makeSkillDir(t, "test-skill", map[string]string{
		"SKILL.md":     "---\nname: test-skill\n---\nEntry.",
		"deep-dive.md": "# Deep Dive\n\nThis is the full body of the deep-dive node.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	node, ok := reg.ReadSkillNode("test-skill", "deep-dive")
	if !ok {
		t.Fatal("expected deep-dive node to be found")
	}
	if !strings.Contains(node.Instructions, "full body of the deep-dive node") {
		t.Errorf("expected full body in instructions, got %q", node.Instructions)
	}
}

func TestReadSkillNode_MissingSkill_ReturnsFalse(t *testing.T) {
	reg := skills.NewRegistry()
	_, ok := reg.ReadSkillNode("nonexistent", "some-node")
	if ok {
		t.Error("expected false for nonexistent skill")
	}
}

func TestReadSkillNode_MissingNode_ReturnsFalse(t *testing.T) {
	root := makeSkillDir(t, "minimal", map[string]string{
		"SKILL.md": "---\nname: minimal\n---\n",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)
	_, ok := reg.ReadSkillNode("minimal", "does-not-exist")
	if ok {
		t.Error("expected false for missing node")
	}
}

// ─────────────────────────────────────────────
// 4. BuildContext - Map of Content format
// ─────────────────────────────────────────────

func TestBuildContext_ContainsOnlyNodeSummaries(t *testing.T) {
	root := makeSkillDir(t, "legal", map[string]string{
		"SKILL.md":           "---\nname: legal\ndescription: Legal compliance assistant\n---\n# Legal AI Assistant\nFull legal instructions that are very long...\nLine 2...\nLine 3...\nLine 4...\n",
		"risk-management.md": "---\nsummary: Managing legal risk across jurisdictions\n---\nVery detailed risk management instructions spanning many pages of legal text.",
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)

	ctx := reg.BuildContext([]string{"legal"})

	// Should contain the skill name
	if !strings.Contains(ctx, `name="legal"`) {
		t.Errorf("expected skill name in context")
	}
	// Should contain node names
	if !strings.Contains(ctx, `name="SKILL"`) {
		t.Errorf("expected SKILL node in Map of Content")
	}
	if !strings.Contains(ctx, `name="risk-management"`) {
		t.Errorf("expected risk-management node in Map of Content")
	}
	// Should contain summaries, not full body
	if !strings.Contains(ctx, "Managing legal risk across jurisdictions") {
		t.Errorf("expected node summary in context")
	}
	// Should NOT contain the full node instructions
	if strings.Contains(ctx, "Very detailed risk management instructions spanning many pages") {
		t.Errorf("BuildContext leaked full node body — should only include summaries")
	}
}

// ─────────────────────────────────────────────
// 5. Context reduction ratio test
// ─────────────────────────────────────────────

func TestContextReduction_MeasuresTokenReduction(t *testing.T) {
	// Simulate a large skill with multiple nodes
	longBody := strings.Repeat("This is a detailed legal instruction that matters greatly. ", 50)
	root := makeSkillDir(t, "law", map[string]string{
		"SKILL.md":          "---\nname: law\ndescription: Full service legal assistant\n---\n" + longBody,
		"contracts.md":      "---\nsummary: Handling contracts and agreements\n---\n" + longBody,
		"litigation.md":     "---\nsummary: Managing litigation and court proceedings\n---\n" + longBody,
		"ip-law.md":         "---\nsummary: Intellectual property rights and filings\n---\n" + longBody,
		"compliance.md":     "---\nsummary: Regulatory compliance and auditing\n---\n" + longBody,
		"employment-law.md": "---\nsummary: Employment and labor law\n---\n" + longBody,
	})
	reg := skills.NewRegistry()
	_ = reg.LoadAll(context.Background(), root)

	// Old approach: full skill text
	fullContent := ""
	sk, _ := reg.Get("law")
	for _, node := range sk.Nodes {
		fullContent += node.Instructions + "\n"
	}

	// New approach: Map of Content
	mapOfContent := reg.BuildContext([]string{"law"})

	reduction := 1.0 - float64(len(mapOfContent))/float64(len(fullContent))
	t.Logf("Full content size: %d chars", len(fullContent))
	t.Logf("Map of Content size: %d chars", len(mapOfContent))
	t.Logf("Context reduction: %.1f%%", reduction*100)

	if reduction < 0.80 {
		t.Errorf("expected at least 80%% context reduction, got %.1f%%", reduction*100)
	}
}
