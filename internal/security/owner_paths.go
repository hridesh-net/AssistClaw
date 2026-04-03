package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathsTouchedByTool lists filesystem paths a tool call may create or modify.
// stateDir and workspaceDir are used to resolve relative paths (plus process cwd).
func PathsTouchedByTool(toolName, inputJSON, stateDir, workspaceDir string) []string {
	switch toolName {
	case "write_file", "edit":
		var m struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(inputJSON), &m) != nil || m.Path == "" {
			return nil
		}
		return resolvePathCandidates(m.Path, stateDir, workspaceDir)
	case "apply_patch":
		var m struct {
			Patch string `json:"patch"`
			Dir   string `json:"dir"`
		}
		if json.Unmarshal([]byte(inputJSON), &m) != nil {
			return nil
		}
		return pathsFromUnifiedDiff(m.Patch, m.Dir, stateDir, workspaceDir)
	case "env":
		var m struct {
			Command string `json:"command"`
			Path    string `json:"path"`
		}
		if json.Unmarshal([]byte(inputJSON), &m) != nil {
			return nil
		}
		if strings.ToLower(strings.TrimSpace(m.Command)) != "write_file" {
			return nil
		}
		p := strings.TrimSpace(m.Path)
		if p == "" {
			p = ".env"
		}
		return resolvePathCandidates(p, stateDir, workspaceDir)
	case "bash":
		var m struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(inputJSON), &m) != nil || strings.TrimSpace(m.Command) == "" {
			return nil
		}
		return nil // bash handled in Guardrail with owner-rule list (see bashTouchesOwnerAbsPaths).
	default:
		return nil
	}
}

// bashTouchesOwnerAbsPaths returns absolute paths under stateDir that appear in the shell
// command. If any match owner-only rules, the guardrail blocks the call (agent should use
// read_file for reads instead of bash).
func bashTouchesOwnerAbsPaths(cmd, stateDir string, relRules []string) []string {
	if stateDir == "" || len(relRules) == 0 {
		return nil
	}
	stateDir = filepath.Clean(stateDir)
	seen := map[string]bool{}
	var out []string
	for _, rule := range relRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		clean := filepath.Clean(rule)
		abs := filepath.Join(stateDir, clean)
		abs = filepath.Clean(abs)
		if !strings.HasPrefix(abs, stateDir) {
			continue
		}
		if strings.Contains(cmd, abs) && !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolvePathCandidates(raw, stateDir, workspaceDir string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" || p == "." {
			return
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if filepath.IsAbs(raw) {
		add(raw)
		return out
	}
	if stateDir != "" {
		add(filepath.Join(stateDir, raw))
	}
	if workspaceDir != "" && workspaceDir != stateDir {
		add(filepath.Join(workspaceDir, raw))
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, raw))
	}
	return out
}

func pathsFromUnifiedDiff(patch, dir, stateDir, workspaceDir string) []string {
	var rels []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "+++ ") {
			continue
		}
		path := strings.TrimPrefix(line, "+++ ")
		path = strings.TrimPrefix(path, "b/")
		if idx := strings.Index(path, "\t"); idx > 0 {
			path = path[:idx]
		}
		path = strings.TrimSpace(path)
		if path == "" || path == "/dev/null" {
			continue
		}
		rels = append(rels, path)
	}
	if len(rels) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, rel := range rels {
		if filepath.IsAbs(rel) {
			p := filepath.Clean(rel)
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			continue
		}
		if dir != "" {
			p := filepath.Clean(filepath.Join(dir, rel))
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
		for _, c := range resolvePathCandidates(rel, stateDir, workspaceDir) {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// AbsMatchesOwnerOnly reports whether absPath is a file or inside a directory
// protected by relRules (paths relative to stateDir). Rules use slash-separated form.
func AbsMatchesOwnerOnly(stateDir, absPath string, relRules []string) bool {
	if stateDir == "" || len(relRules) == 0 {
		return false
	}
	stateDir = filepath.Clean(stateDir)
	absPath = filepath.Clean(absPath)
	rel, err := filepath.Rel(stateDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return false
	}
	for _, rule := range relRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		rule = filepath.ToSlash(filepath.Clean(rule))
		if rule == "" {
			continue
		}
		if isDirRule(rule) {
			rule = strings.TrimSuffix(rule, "/")
			if rel == rule || strings.HasPrefix(rel, rule+"/") {
				return true
			}
			continue
		}
		if rel == rule {
			return true
		}
	}
	return false
}

func isDirRule(rule string) bool {
	rule = strings.TrimSuffix(strings.TrimSpace(rule), "/")
	if rule == "" {
		return false
	}
	// Paths with a file extension (e.g. .md, .yaml) are single files; others are directory trees.
	return filepath.Ext(rule) == ""
}

// OwnerOnlyBlockMessage returns a user-facing error when a tool targets owner-only paths.
func OwnerOnlyBlockMessage(toolName string, paths []string) string {
	if len(paths) == 0 {
		return fmt.Sprintf("Tool %q blocked: target is owner-only (policies / rules). Edit those files on the host as the human operator, not via the agent.", toolName)
	}
	return fmt.Sprintf("Tool %q blocked: cannot modify owner-only path(s): %s — only the human operator may change POLICIES.md, RULES.md, or files under policies/.", toolName, strings.Join(paths, ", "))
}
