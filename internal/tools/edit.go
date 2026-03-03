package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// EditFileTool performs targeted in-place string replacements in a file.
// This is one of the most heavily-used coding tools — it lets the LLM make
// precise, surgical edits without rewriting the entire file.
type EditFileTool struct{}

func (EditFileTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "edit",
		Description: `Make a precise edit to an existing file by replacing an exact string with new content.
Use this instead of write_file when you only need to change part of a file.
Rules:
- old_str MUST exactly match text in the file (including whitespace/indentation).
- To insert new content, set old_str to the unique line just before the insertion point and include that line in new_str.
- For multiple separate edits to the same file, call this tool multiple times.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute or relative path to the file"},
				"old_str": map[string]any{"type": "string", "description": "Exact string to find and replace (must match exactly, including whitespace)"},
				"new_str": map[string]any{"type": "string", "description": "Replacement string"},
			},
			Required: []string{"path", "old_str", "new_str"},
		},
	}
}

func (EditFileTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path   string `json:"path"`
		OldStr string `json:"old_str"`
		NewStr string `json:"new_str"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("edit: cannot read %s: %w", args.Path, err)
	}

	content := string(data)
	count := strings.Count(content, args.OldStr)
	if count == 0 {
		// Provide a helpful diff-style error
		return fmt.Sprintf(
			"edit: no match found for old_str in %s.\n\nExpected to find:\n%s\n\nMake sure the indentation and whitespace match exactly.",
			args.Path, args.OldStr,
		), nil
	}
	if count > 1 {
		return fmt.Sprintf(
			"edit: old_str matches %d locations in %s. Make old_str more specific so it matches exactly once.",
			count, args.Path,
		), nil
	}

	newContent := strings.Replace(content, args.OldStr, args.NewStr, 1)
	if err := os.WriteFile(args.Path, []byte(newContent), 0o644); err != nil {
		return "", fmt.Errorf("edit: cannot write %s: %w", args.Path, err)
	}

	// Count changed lines for feedback
	oldLines := len(strings.Split(args.OldStr, "\n"))
	newLines := len(strings.Split(args.NewStr, "\n"))
	return fmt.Sprintf("✔ edit applied to %s (-%d +%d lines)", args.Path, oldLines, newLines), nil
}
