package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// ApplyPatchTool applies a unified diff (patch) string to files.
// This enables surgical multi-file edits from the model.
type ApplyPatchTool struct{}

func (ApplyPatchTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "apply_patch",
		Description: `Apply a unified diff (patch) to one or more files.
The patch must be in standard 'diff -u' format with '--- a/file', '+++ b/file' headers.
Use this for multi-file, multi-hunk edits when the 'edit' tool would require too many calls.
Returns success message or patch error output to help diagnose mismatches.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "Unified diff content (output of 'diff -u original new')",
				},
				"dir": map[string]any{
					"type":        "string",
					"description": "Working directory to apply the patch in (optional, defaults to current dir)",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "If true, check the patch without modifying files",
				},
			},
			Required: []string{"patch"},
		},
	}
}

func (ApplyPatchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Patch  string `json:"patch"`
		Dir    string `json:"dir"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Patch) == "" {
		return "apply_patch: patch content is empty", nil
	}

	// Try the `patch` binary first (most reliable)
	patchBin, err := exec.LookPath("patch")
	if err == nil {
		return applyWithPatchBin(ctx, patchBin, args.Patch, args.Dir, args.DryRun)
	}

	// Fallback: pure-Go single-file simple patch (handles basic +/- hunks)
	return applyPurGoFallback(args.Patch, args.Dir)
}

func applyWithPatchBin(ctx context.Context, bin, patch, dir string, dryRun bool) (string, error) {
	patchArgs := []string{"-p1", "--quiet"}
	if dryRun {
		patchArgs = append(patchArgs, "--dry-run")
	}

	// Write patch to a temp file
	tmp, err := os.CreateTemp("", "assistclaw-patch-*.patch")
	if err != nil {
		return "", fmt.Errorf("apply_patch: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(patch); err != nil {
		return "", fmt.Errorf("apply_patch: %w", err)
	}
	tmp.Close()

	patchArgs = append(patchArgs, "-i", tmp.Name())
	cmd := exec.CommandContext(ctx, bin, patchArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))

	if err != nil {
		if result != "" {
			return fmt.Sprintf("apply_patch failed:\n%s", result), nil
		}
		return fmt.Sprintf("apply_patch failed: %v", err), nil
	}
	if dryRun {
		return "✔ Patch applies cleanly (dry-run — no files modified)", nil
	}
	if result != "" {
		return fmt.Sprintf("✔ Patch applied successfully.\n%s", result), nil
	}
	return "✔ Patch applied successfully.", nil
}

// applyPurGoFallback handles the simplest case: single-file patch with no `patch` binary.
func applyPurGoFallback(patch, dir string) (string, error) {
	lines := strings.Split(patch, "\n")
	var targetFile string
	var hunks []string
	var currentHunk strings.Builder

	inHunk := false
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") {
			// Extract file path
			path := strings.TrimPrefix(line, "+++ ")
			// Remove "b/" prefix from unified diff
			path = strings.TrimPrefix(path, "b/")
			// Remove timestamp if present
			if idx := strings.Index(path, "\t"); idx > 0 {
				path = path[:idx]
			}
			targetFile = strings.TrimSpace(path)
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if inHunk && currentHunk.Len() > 0 {
				hunks = append(hunks, currentHunk.String())
				currentHunk.Reset()
			}
			inHunk = true
			continue
		}
		if inHunk {
			currentHunk.WriteString(line + "\n")
		}
	}
	if currentHunk.Len() > 0 {
		hunks = append(hunks, currentHunk.String())
	}

	if targetFile == "" || len(hunks) == 0 {
		return "apply_patch: could not parse patch — ensure it is unified diff format with +++ b/path header", nil
	}

	filePath := targetFile
	if dir != "" {
		filePath = dir + "/" + targetFile
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("apply_patch: cannot read %s: %v", filePath, err), nil
	}
	content := string(data)

	for _, hunk := range hunks {
		// Find old lines (lines starting with '-' or ' ')
		var oldParts, newParts []string
		for _, l := range strings.Split(hunk, "\n") {
			if strings.HasPrefix(l, "-") {
				oldParts = append(oldParts, strings.TrimPrefix(l, "-"))
			} else if strings.HasPrefix(l, "+") {
				newParts = append(newParts, strings.TrimPrefix(l, "+"))
			} else if strings.HasPrefix(l, " ") {
				// Context line: appears in both
				oldParts = append(oldParts, strings.TrimPrefix(l, " "))
				newParts = append(newParts, strings.TrimPrefix(l, " "))
			}
		}
		oldBlock := strings.Join(oldParts, "\n")
		newBlock := strings.Join(newParts, "\n")
		content = strings.Replace(content, oldBlock, newBlock, 1)
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("apply_patch: cannot write %s: %v", filePath, err), nil
	}
	return fmt.Sprintf("✔ Patch applied to %s (%d hunk(s)) via built-in fallback.", targetFile, len(hunks)), nil
}
