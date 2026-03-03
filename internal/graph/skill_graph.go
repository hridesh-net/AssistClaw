package graph

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// SkillEdgeType classifies relationships between skill nodes.
type SkillEdgeType int

const (
	SkillEdgeEntry    SkillEdgeType = iota // skill root → SKILL.md
	SkillEdgeExtends                       // wikilink [[node]] in body
	SkillEdgeRequires                      // frontmatter: requires: [node]
	SkillEdgeExample                       // frontmatter: examples: [node]
	SkillEdgeTool                          // skill-defined tool (tools: block)
)

var skillEdgeLabel = map[SkillEdgeType]string{
	SkillEdgeEntry:    "ENTRY",
	SkillEdgeExtends:  "EXTENDS",
	SkillEdgeRequires: "REQUIRES",
	SkillEdgeExample:  "EXAMPLE",
	SkillEdgeTool:     "TOOL",
}

// SkillNodeKey uniquely identifies a node as "skill/node".
type SkillNodeKey struct {
	Skill, Node string
}

// SkillNode is a node in the skill graph.
type SkillNode struct {
	Skill   string
	Name    string
	Summary string
	// Links are parsed [[wikilink]] targets from the node body.
	Links []string
	// ToolNames lists skill-defined tool names (from SKILL.md tools: block).
	ToolNames []string
}

// SkillEdge is a directed edge in the skill graph.
type SkillEdge struct {
	From SkillNodeKey
	To   SkillNodeKey
	Type SkillEdgeType
}

// wikilinkRe matches wikilinks like [[node-name]] or [[ node name ]].
var wikilinkRe = regexp.MustCompile(`\[\[\s*([^\]]+?)\s*\]\]`)

// SkillGraph is a directed graph over skill nodes and their relationships.
type SkillGraph struct {
	mu    sync.RWMutex
	nodes map[SkillNodeKey]*SkillNode
	edges []SkillEdge
}

// NewSkillGraph creates an empty skill graph.
func NewSkillGraph() *SkillGraph {
	return &SkillGraph{
		nodes: make(map[SkillNodeKey]*SkillNode),
	}
}

// AddSkill registers a skill with all its nodes and parses wikilink edges.
// skillName is the skill directory name. nodes is a map of nodeName → (summary, body, toolNames).
func (g *SkillGraph) AddSkill(skillName string, nodes map[string]SkillNodeInput) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// First pass: add all nodes
	for nodeName, inp := range nodes {
		key := SkillNodeKey{skillName, nodeName}
		links := extractWikilinks(inp.Body)
		g.nodes[key] = &SkillNode{
			Skill:     skillName,
			Name:      nodeName,
			Summary:   inp.Summary,
			Links:     links,
			ToolNames: inp.ToolNames,
		}

		// ENTRY edge: skill → SKILL node
		if nodeName == "SKILL" {
			g.edges = append(g.edges, SkillEdge{
				From: SkillNodeKey{skillName, "_root"},
				To:   key,
				Type: SkillEdgeEntry,
			})
		}
	}

	// Second pass: EXTENDS edges from wikilinks
	for nodeName, inp := range nodes {
		from := SkillNodeKey{skillName, nodeName}
		for _, link := range extractWikilinks(inp.Body) {
			to := SkillNodeKey{skillName, link}
			if _, exists := g.nodes[to]; exists {
				g.edges = append(g.edges, SkillEdge{
					From: from,
					To:   to,
					Type: SkillEdgeExtends,
				})
			}
		}
		// TOOL edges
		for _, toolName := range inp.ToolNames {
			g.edges = append(g.edges, SkillEdge{
				From: SkillNodeKey{skillName, nodeName},
				To:   SkillNodeKey{skillName, "_tool:" + toolName},
				Type: SkillEdgeTool,
			})
		}
		_ = inp
	}
}

// SkillNodeInput holds the input data for a single skill node.
type SkillNodeInput struct {
	Summary   string
	Body      string
	ToolNames []string
}

// Neighbors returns all directly linked nodes from a given skill/node.
func (g *SkillGraph) Neighbors(skill, node string) []SkillEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	from := SkillNodeKey{skill, node}
	var out []SkillEdge
	for _, e := range g.edges {
		if e.From == from {
			out = append(out, e)
		}
	}
	return out
}

// Node returns a skill node by skill/node key.
func (g *SkillGraph) Node(skill, node string) (*SkillNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[SkillNodeKey{skill, node}]
	return n, ok
}

// Index returns a compact text index of all active skill graphs for the system prompt.
// Format: "github: [[SKILL]], [[create-pr]] | notion: [[SKILL]]"
// Designed to be <100 tokens regardless of skill count.
func (g *SkillGraph) Index(activeSkills []string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(activeSkills) == 0 {
		return ""
	}

	var parts []string
	for _, skill := range activeSkills {
		var nodeNames []string
		for key, node := range g.nodes {
			if key.Skill == skill && node.Name != "" && !strings.HasPrefix(node.Name, "_") {
				nodeNames = append(nodeNames, "[["+node.Name+"]]")
			}
		}
		if len(nodeNames) == 0 {
			continue
		}
		// SKILL always first
		sorted := make([]string, 0, len(nodeNames))
		for _, n := range nodeNames {
			if n == "[[SKILL]]" {
				sorted = append([]string{n}, sorted...)
			} else {
				sorted = append(sorted, n)
			}
		}
		parts = append(parts, fmt.Sprintf("%s: %s", skill, strings.Join(sorted, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// FormatEdges returns the wikilink edge list for a node's output (shown after node body).
func (g *SkillGraph) FormatEdges(skill, node string) string {
	neighbors := g.Neighbors(skill, node)
	if len(neighbors) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n---\n**Links from this node:**\n")
	for _, e := range neighbors {
		// skip _tool and _root pseudo-nodes
		if strings.HasPrefix(e.To.Node, "_") {
			continue
		}
		n, ok := g.nodes[e.To]
		label := skillEdgeLabel[e.Type]
		if ok && n.Summary != "" {
			sb.WriteString(fmt.Sprintf("- [[%s]] (%s) — %s\n", e.To.Node, label, n.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("- [[%s]] (%s)\n", e.To.Node, label))
		}
	}
	sb.WriteString(fmt.Sprintf("\nCall `read_skill_node(\"%s\", \"<node>\")` to follow any link.", skill))
	return sb.String()
}

// extractWikilinks parses [[wikilinks]] from markdown body text.
func extractWikilinks(body string) []string {
	matches := wikilinkRe.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}
