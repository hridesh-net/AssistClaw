package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// ReadSkillNodeTool allows the agent to lazily load a specific node from a Skill Graph.
// After returning the node body it also appends the outgoing [[wikilink]] edges so
// the agent knows where it can navigate next without a separate index call.
type ReadSkillNodeTool struct {
	registry Registry
}

var _ agent.Tool = (*ReadSkillNodeTool)(nil)

func NewReadSkillNodeTool(registry Registry) *ReadSkillNodeTool {
	return &ReadSkillNodeTool{registry: registry}
}

func (t *ReadSkillNodeTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "read_skill_node",
		Description: "Reads the full content of a specific node within a Skill Graph. After the node body, outgoing [[wikilinks]] are listed so you can navigate the graph. Start with SKILL node if unsure.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"skill_name": provider.ToolParameter{
					Type:        "string",
					Description: "The name of the skill graph (e.g. 'skill-creator')",
				},
				"node_name": provider.ToolParameter{
					Type:        "string",
					Description: "The name of the node to read (e.g. 'init_skill' or 'SKILL')",
				},
			},
			Required: []string{"skill_name", "node_name"},
		},
	}
}

func (t *ReadSkillNodeTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		SkillName string `json:"skill_name"`
		NodeName  string `json:"node_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	node, ok := t.registry.ReadSkillNode(args.SkillName, args.NodeName)
	if !ok {
		return "", fmt.Errorf("skill node '%s/%s' not found", args.SkillName, args.NodeName)
	}

	body := fmt.Sprintf("## Node: %s/%s\n\n%s", args.SkillName, node.Name, node.Instructions)

	// Append outgoing wikilink edges from the skill registry so the agent
	// can navigate the graph without needing to call skill_graph_index again.
	links := extractWikilinksFromBody(node.Instructions)
	if len(links) > 0 {
		skill, ok := t.registry.Get(args.SkillName)
		if ok {
			var sb strings.Builder
			sb.WriteString("\n\n---\n**Links from this node:**\n")
			for _, link := range links {
				if linkedNode, exists := skill.Nodes[link]; exists {
					summary := linkedNode.Summary
					if summary != "" {
						sb.WriteString(fmt.Sprintf("- [[%s]] — %s\n", link, summary))
					} else {
						sb.WriteString(fmt.Sprintf("- [[%s]]\n", link))
					}
				}
			}
			sb.WriteString(fmt.Sprintf("\nCall `read_skill_node(\"%s\", \"<node>\")` to follow any link.", args.SkillName))
			body += sb.String()
		}
	}

	return body, nil
}

// extractWikilinksFromBody extracts [[wikilink]] targets from a node body.
func extractWikilinksFromBody(body string) []string {
	// reuse the regex from skill_graph.go via a simple inline parser
	var links []string
	seen := map[string]bool{}
	i := 0
	for i < len(body)-3 {
		if body[i] == '[' && body[i+1] == '[' {
			end := strings.Index(body[i+2:], "]]")
			if end >= 0 {
				link := strings.TrimSpace(body[i+2 : i+2+end])
				if link != "" && !seen[link] {
					seen[link] = true
					links = append(links, link)
				}
				i = i + 2 + end + 2
				continue
			}
		}
		i++
	}
	return links
}
