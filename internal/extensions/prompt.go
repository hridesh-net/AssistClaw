// Package extensions provides optional hooks for merging prompt fragments into the system prompt.
// (workspace prompt fragments, etc.) without embedding a Node plugin runtime.
package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxPromptFileBytes = 16 * 1024
	maxPromptTotal     = 64 * 1024
)

// PromptAppendix loads text/markdown files listed in config and returns a block to append
// to the agent system prompt. Paths are relative to stateDir when not absolute.
func PromptAppendix(stateDir string, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var b strings.Builder
	total := 0
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(stateDir, p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			_, _ = fmt.Fprintf(&b, "\n[assistclaw extensions: skipped %s: %v]\n", p, err)
			continue
		}
		if len(data) > maxPromptFileBytes {
			data = data[:maxPromptFileBytes]
		}
		if total+len(data) > maxPromptTotal {
			b.WriteString("\n[assistclaw extensions: remaining files omitted — global size cap]\n")
			break
		}
		total += len(data)
		_, _ = fmt.Fprintf(&b, "\n## Extension file: %s\n\n", filepath.Base(p))
		b.WriteString(strings.TrimSpace(string(data)))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return "## Extension context (assistclaw.yaml → extensions.prompt_files)\n" + b.String()
}
