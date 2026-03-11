// Package tools implements AssistClaw's built-in tools.
// Each tool implements the agent.Tool interface.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// ─────────────────────────────────────────────
// File tools
// ─────────────────────────────────────────────

// ReadFileTool reads a file from the filesystem.
type ReadFileTool struct{}

func (ReadFileTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "read_file",
		Description: "Read the full contents of a file. Use for source code, configs, logs, docs.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path":       map[string]any{"type": "string", "description": "Absolute or relative path to the file"},
				"start_line": map[string]any{"type": "integer", "description": "Start line (1-indexed, optional)"},
				"end_line":   map[string]any{"type": "integer", "description": "End line (1-indexed, inclusive, optional)"},
			},
			Required: []string{"path"},
		},
	}
}

func (ReadFileTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if args.StartLine > 0 || args.EndLine > 0 {
		lines := strings.Split(content, "\n")
		start, end := args.StartLine-1, args.EndLine
		if start < 0 {
			start = 0
		}
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return content, nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

func (WriteFileTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "write_file",
		Description: "Write content to a file. Creates parent directories if needed. Overwrites existing file.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path":    map[string]any{"type": "string", "description": "Path to write"},
				"content": map[string]any{"type": "string", "description": "File content"},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (WriteFileTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(args.Path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Written %d bytes to %s", len(args.Content), args.Path), nil
}

// ListDirTool lists directory contents.
type ListDirTool struct{}

func (ListDirTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "list_dir",
		Description: "List the contents of a directory.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path":      map[string]any{"type": "string", "description": "Directory path"},
				"recursive": map[string]any{"type": "boolean", "description": "List recursively"},
			},
			Required: []string{"path"},
		},
	}
}

func (ListDirTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	var entries []string
	if args.Recursive {
		err := filepath.WalkDir(args.Path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			entries = append(entries, p)
			return nil
		})
		if err != nil {
			return "", err
		}
	} else {
		des, err := os.ReadDir(args.Path)
		if err != nil {
			return "", err
		}
		for _, de := range des {
			prefix := " "
			if de.IsDir() {
				prefix = "d"
			}
			entries = append(entries, prefix+" "+de.Name())
		}
	}
	return strings.Join(entries, "\n"), nil
}

// GrepTool searches for patterns in files.
type GrepTool struct{}

func (GrepTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "grep",
		Description: "Search for a pattern in files. Returns matching lines with file and line number.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"pattern":     map[string]any{"type": "string", "description": "Search pattern (regex supported)"},
				"path":        map[string]any{"type": "string", "description": "File or directory to search"},
				"recursive":   map[string]any{"type": "boolean", "description": "Search recursively"},
				"ignore_case": map[string]any{"type": "boolean", "description": "Case-insensitive search"},
			},
			Required: []string{"pattern", "path"},
		},
	}
}

func (GrepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Recursive  bool   `json:"recursive"`
		IgnoreCase bool   `json:"ignore_case"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	cmdArgs := []string{"-n", "--color=never"}
	if args.Recursive {
		cmdArgs = append(cmdArgs, "-r")
	}
	if args.IgnoreCase {
		cmdArgs = append(cmdArgs, "-i")
	}
	cmdArgs = append(cmdArgs, args.Pattern, args.Path)
	cmd := exec.CommandContext(ctx, "grep", cmdArgs...)
	out, _ := cmd.CombinedOutput()
	return string(out), nil
}

// ─────────────────────────────────────────────
// Bash execution tool
// ─────────────────────────────────────────────

// BashTool executes shell commands with a timeout.
type BashTool struct {
	// WorkDir is the default working directory. If empty, uses CWD.
	WorkDir string
	// MaxTimeout caps the execution time. Default 30s.
	MaxTimeout time.Duration
}

func (t BashTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "bash",
		Description: "Execute a shell command and return its combined stdout+stderr. Prefer specific commands over shell scripts. Use for file operations, running tests, installing packages, etc.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"command":     map[string]any{"type": "string", "description": "Shell command to execute"},
				"timeout_s":   map[string]any{"type": "integer", "description": "Timeout in seconds (default 30, max 300)"},
				"working_dir": map[string]any{"type": "string", "description": "Working directory (optional)"},
			},
			Required: []string{"command"},
		},
	}
}

func (tool BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command    string `json:"command"`
		TimeoutS   int    `json:"timeout_s"`
		WorkingDir string `json:"working_dir"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	timeout := time.Duration(args.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxTimeout := tool.MaxTimeout
	if maxTimeout <= 0 {
		maxTimeout = 300 * time.Second
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
	cmd.Env = os.Environ()
	if args.WorkingDir != "" {
		cmd.Dir = args.WorkingDir
	} else if tool.WorkDir != "" {
		cmd.Dir = tool.WorkDir
	}

	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		if len(result) > 0 {
			return result, nil // Return output even on failure — LLM needs it to reason
		}
		return fmt.Sprintf("Error: %v", err), nil
	}
	return result, nil
}

// ─────────────────────────────────────────────
// Web fetch tool
// ─────────────────────────────────────────────

// WebFetchTool fetches content from a URL and returns readable text.
// Pairs naturally with web_search: search finds URLs, web_fetch reads them.
type WebFetchTool struct{}

func (WebFetchTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "web_fetch",
		Description: "Fetch the text content of a URL. Strips HTML to readable text by default. " +
			"Use after web_search to read full pages, docs, GitHub files, or any public URL. " +
			"Also works for raw API endpoints that return JSON or plain text.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to fetch (http or https)",
				},
				"max_chars": map[string]any{
					"type":        "integer",
					"description": "Max characters to return (default 8000, max 32000)",
				},
				"raw": map[string]any{
					"type":        "boolean",
					"description": "Return raw HTML/content without stripping tags (default false)",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL      string `json:"url"`
		MaxChars int    `json:"max_chars"`
		Raw      bool   `json:"raw"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.MaxChars <= 0 {
		args.MaxChars = 8000
	}
	if args.MaxChars > 32000 {
		args.MaxChars = 32000
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	curlArgs := []string{"-fsSL", "--max-filesize", "4194304",
		"-H", "User-Agent: Mozilla/5.0 (compatible; AssistClaw/3.5)",
		"--compressed", args.URL}
	cmd := exec.CommandContext(ctx, "curl", curlArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}

	text := string(out)
	// Strip HTML by default (check for '<' prefix or HTML doctype)
	if !args.Raw && (strings.HasPrefix(strings.TrimSpace(text), "<") || strings.Contains(text[:min(200, len(text))], "<!DOCTYPE")) {
		text = webFetchHTMLToText(text)
	}

	runes := []rune(text)
	truncated := false
	if len(runes) > args.MaxChars {
		runes = runes[:args.MaxChars]
		truncated = true
	}
	result := strings.TrimSpace(string(runes))
	if truncated {
		result += fmt.Sprintf("\n\n[... truncated at %d chars. Use max_chars to increase.]", args.MaxChars)
	}
	return result, nil
}

// webFetchHTMLToText strips HTML tags and script/style blocks.
func webFetchHTMLToText(html string) string {
	// Remove script/style
	for _, tag := range []string{"script", "style", "noscript", "nav", "footer"} {
		open := "<" + tag
		close := "</" + tag + ">"
		for {
			start := strings.Index(strings.ToLower(html), open)
			if start == -1 {
				break
			}
			end := strings.Index(strings.ToLower(html[start:]), close)
			if end == -1 {
				html = html[:start]
				break
			}
			html = html[:start] + " " + html[start+end+len(close):]
		}
	}
	// Strip remaining tags
	var out strings.Builder
	inTag := false
	prevSpace := false
	for _, c := range html {
		if c == '<' {
			inTag = true
			out.WriteRune('\n')
			prevSpace = false
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if inTag {
			continue
		}
		if c == '\r' || c == '\t' {
			c = ' '
		}
		if c == ' ' {
			if !prevSpace {
				out.WriteRune(' ')
				prevSpace = true
			}
		} else {
			out.WriteRune(c)
			prevSpace = (c == '\n')
		}
	}
	text := out.String()
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────
// Memory search tool
// ─────────────────────────────────────────────

// MemorySearchTool searches the episodic memory store.
type MemorySearchTool struct {
	SearchFn func(ctx context.Context, query string, limit int) ([]string, error)
}

func (t MemorySearchTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "memory_search",
		Description: "Search past conversations and indexed documents for relevant content.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
				"limit": map[string]any{"type": "integer", "description": "Number of results (default 10)"},
			},
			Required: []string{"query"},
		},
	}
}

func (t MemorySearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query    string  `json:"query"`
		Limit    int     `json:"limit"`
		MinScore float32 `json:"min_score"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Limit <= 0 {
		args.Limit = 10
	}
	results, err := t.SearchFn(ctx, args.Query, args.Limit)
	if err != nil {
		return fmt.Sprintf("memory search error: %v", err), nil
	}

	// Note: t.SearchFn currently returns episodic results.
	// The implementation in main.go handles merging episodic and semantic.
	// We'll rely on that for now, but we've added MinScore placeholder for semantic filtering.

	if len(results) == 0 {
		return "No results found.", nil
	}
	return strings.Join(results, "\n---\n"), nil
}

// ─────────────────────────────────────────────
// Memory get tool
// ─────────────────────────────────────────────

// MemoryGetTool reads specific lines from a memory file.
type MemoryGetTool struct {
	SnippetFn func(ctx context.Context, source string, startLine, endLine int) (string, error)
}

func (t MemoryGetTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "memory_get",
		Description: "Read a specific line range from a memory file (MEMORY.md or memory/*.md). Use after memory_search to pull only the needed lines.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path":       map[string]any{"type": "string", "description": "Path to the file (from memory_search results)"},
				"start_line": map[string]any{"type": "integer", "description": "Start line (1-indexed)"},
				"end_line":   map[string]any{"type": "integer", "description": "End line (inclusive)"},
			},
			Required: []string{"path"},
		},
	}
}

func (t MemoryGetTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	content, err := t.SnippetFn(ctx, args.Path, args.StartLine, args.EndLine)
	if err != nil {
		return fmt.Sprintf("memory get error: %v", err), nil
	}
	return content, nil
}

// ─────────────────────────────────────────────
// All returns all default built-in tools.
// ─────────────────────────────────────────────

// Default registers all built-in tools into a registry.
func Default(
	memSearchFn func(ctx context.Context, query string, limit int) ([]string, error),
	memSnippetFn func(ctx context.Context, source string, startLine, endLine int) (string, error),
	episodic *memory.EpisodicMemory,
	visionProvider provider.Provider,
	visionModel string,
	channelSenders map[string]ChannelSender,
	stateDir string,
) []interface{ Definition() provider.ToolDef } {
	return []interface{ Definition() provider.ToolDef }{
		// ── Tier 0: original tools ──────────────────────
		ReadFileTool{},
		WriteFileTool{},
		ListDirTool{},
		GrepTool{},
		BashTool{MaxTimeout: 300 * time.Second},
		WebFetchTool{},
		MemorySearchTool{SearchFn: memSearchFn},
		MemoryGetTool{SnippetFn: memSnippetFn},
		BrowserNavigate{},
		BrowserScreenshot{},

		// ── Tier 1: new core tools ───────────────────────
		EditFileTool{},
		WebSearchTool{},
		ProcessTool{PersistencePath: filepath.Join(stateDir, "processes.json")},
		ApplyPatchTool{},
		EnvTool{},

		// ── Tier 2: intelligence tools ───────────────────
		ImageUnderstandTool{Provider: visionProvider, Model: visionModel},
		SessionsListTool{Episodic: episodic},
		SessionsHistoryTool{Episodic: episodic},
		CronTool{PersistencePath: filepath.Join(stateDir, "cron_jobs.json")},
		MessageTool{Senders: channelSenders},
		SendMediaTool{},
		ListHardwareTool{},
	}
}
