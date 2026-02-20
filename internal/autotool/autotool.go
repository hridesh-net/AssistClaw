// Package autotool implements autonomous tool generation — the ability to
// draft, sandbox-test, safety-validate, and persist new Python tools at runtime.
package autotool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ─────────────────────────────────────────────
// Safety policy
// ─────────────────────────────────────────────

// forbiddenPatterns are patterns that auto-generated tools must NOT contain.
// This is a defense-in-depth measure; human approval is still required to persist.
var forbiddenPatterns = []string{
	"subprocess.run.*shell=True",
	"os.system(",
	"eval(",
	"exec(",
	"__import__(",
	"sudo",
	"/etc/passwd",
	"/etc/shadow",
	"~/.ssh",
	"~/.assistclaw/credentials",
	"/root",
}

// ValidateScript checks a Python script against the safety policy.
// Returns a non-nil error with a human-readable explanation if unsafe.
func ValidateScript(script string) error {
	lower := strings.ToLower(script)
	for _, pattern := range forbiddenPatterns {
		p := strings.ToLower(pattern)
		if strings.Contains(lower, strings.ReplaceAll(p, ".*", "")) {
			return fmt.Errorf("safety policy violation: script contains forbidden pattern %q", pattern)
		}
	}
	return nil
}

// ─────────────────────────────────────────────
// Tool metadata
// ─────────────────────────────────────────────

// ToolMeta describes a persisted auto-generated tool.
type ToolMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ScriptPath  string    `json:"script_path"`
	CreatedAt   time.Time `json:"created_at"`
	// Args describes the command-line arguments the script accepts.
	Args []ArgDef `json:"args"`
}

// ArgDef describes one argument of an auto-generated tool.
type ArgDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ─────────────────────────────────────────────
// Registry
// ─────────────────────────────────────────────

// Registry manages persisted auto-generated tools on disk.
type Registry struct {
	dir string
	log *zap.Logger
}

// NewRegistry creates a tool registry rooted at dir.
func NewRegistry(dir string, log *zap.Logger) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("autotool registry: mkdir %s: %w", dir, err)
	}
	return &Registry{dir: dir, log: log}, nil
}

// List returns all registered tools.
func (r *Registry) List() ([]ToolMeta, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	var tools []ToolMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		meta, err := r.loadMeta(filepath.Join(r.dir, e.Name()))
		if err != nil {
			r.log.Warn("autotool: skipping corrupt meta", zap.String("file", e.Name()), zap.Error(err))
			continue
		}
		tools = append(tools, meta)
	}
	return tools, nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (ToolMeta, error) {
	metaPath := filepath.Join(r.dir, name+".meta.json")
	meta, err := r.loadMeta(metaPath)
	if err != nil {
		return ToolMeta{}, fmt.Errorf("autotool: get %q: %w", name, err)
	}
	return meta, nil
}

// Save persists a tool to disk. Script content is written to <name>.py,
// metadata to <name>.meta.json.
func (r *Registry) Save(meta ToolMeta, script string) error {
	scriptPath := filepath.Join(r.dir, meta.Name+".py")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return fmt.Errorf("autotool: write script: %w", err)
	}
	meta.ScriptPath = scriptPath
	meta.CreatedAt = time.Now()

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, meta.Name+".meta.json"), metaData, 0o644)
}

// Delete removes a tool from the registry.
func (r *Registry) Delete(name string) error {
	_ = os.Remove(filepath.Join(r.dir, name+".py"))
	_ = os.Remove(filepath.Join(r.dir, name+".meta.json"))
	return nil
}

func (r *Registry) loadMeta(path string) (ToolMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolMeta{}, err
	}
	var meta ToolMeta
	return meta, json.Unmarshal(data, &meta)
}

// ─────────────────────────────────────────────
// Sandbox runner
// ─────────────────────────────────────────────

// SandboxConfig configures the Python sandbox.
type SandboxConfig struct {
	// VenvPath is the Python venv used for sandboxed execution.
	VenvPath string
	// Timeout caps sandbox execution time.
	Timeout time.Duration
	// MaxOutputBytes limits output captured from the script.
	MaxOutputBytes int64
}

// Sandbox runs Python scripts in an isolated venv.
type Sandbox struct {
	cfg SandboxConfig
	log *zap.Logger
}

// NewSandbox creates a sandboxed Python runner.
func NewSandbox(cfg SandboxConfig, log *zap.Logger) *Sandbox {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 1 << 20 // 1MB
	}
	return &Sandbox{cfg: cfg, log: log}
}

// SandboxResult holds the outcome of running a script in the sandbox.
type SandboxResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Run executes script content in the sandbox venv.
// The script is written to a temp file and executed; the temp file is
// removed afterward regardless of the outcome.
func (s *Sandbox) Run(ctx context.Context, script string, args []string) (*SandboxResult, error) {
	// Write script to a temp file.
	tmpDir, err := os.MkdirTemp("", "assistclaw-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("sandbox: mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "tool.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return nil, fmt.Errorf("sandbox: write script: %w", err)
	}

	python := filepath.Join(s.cfg.VenvPath, "bin", "python3")
	if _, err := os.Stat(python); os.IsNotExist(err) {
		python = "python3" // fallback to system python
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, python, cmdArgs...) //nolint:gosec
	cmd.Dir = tmpDir
	// Minimal environment for security.
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + os.Getenv("HOME"),
		"LANG=en_US.UTF-8",
	}

	start := time.Now()
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	output := string(out)
	if int64(len(output)) > s.cfg.MaxOutputBytes {
		output = output[:s.cfg.MaxOutputBytes] + "\n[output truncated]"
	}

	s.log.Info("sandbox executed",
		zap.Duration("duration", dur),
		zap.Int("exit_code", exitCode),
		zap.Int("output_bytes", len(out)),
	)

	return &SandboxResult{
		Stdout:   output,
		ExitCode: exitCode,
		Duration: dur,
	}, nil
}

// EnsureVenv creates a Python venv at path if it doesn't exist.
func EnsureVenv(path string) error {
	if _, err := os.Stat(filepath.Join(path, "bin", "python3")); err == nil {
		return nil // already exists
	}
	cmd := exec.Command("python3", "-m", "venv", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("venv create: %w: %s", err, out)
	}
	return nil
}

// ─────────────────────────────────────────────
// Creator
// ─────────────────────────────────────────────

// CreatorConfig configures the auto-tool creator.
type CreatorConfig struct {
	ToolsDir string
	VenvPath string
	Timeout  time.Duration
}

// Creator orchestrates the full draft → validate → sandbox → confirm → persist pipeline.
type Creator struct {
	cfg      CreatorConfig
	registry *Registry
	sandbox  *Sandbox
	log      *zap.Logger
}

// NewCreator creates a new auto-tool creator.
func NewCreator(cfg CreatorConfig, log *zap.Logger) (*Creator, error) {
	reg, err := NewRegistry(cfg.ToolsDir, log)
	if err != nil {
		return nil, err
	}
	if err := EnsureVenv(cfg.VenvPath); err != nil {
		log.Warn("could not create venv, sandbox may use system Python", zap.Error(err))
	}
	sandbox := NewSandbox(SandboxConfig{
		VenvPath: cfg.VenvPath,
		Timeout:  cfg.Timeout,
	}, log)
	return &Creator{cfg: cfg, registry: reg, sandbox: sandbox, log: log}, nil
}

// DraftResult holds a drafted tool ready for user review.
type DraftResult struct {
	ID          string
	Name        string
	Description string
	Script      string
	SafetyOK    bool
	SafetyError string
	TestResult  *SandboxResult
}

// Draft validates and tests a new tool script drafted by the LLM.
// It does NOT persist the tool — call Persist() after user confirmation.
func (c *Creator) Draft(ctx context.Context, name, description, script string) (*DraftResult, error) {
	result := &DraftResult{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Script:      script,
	}

	// Safety check.
	if err := ValidateScript(script); err != nil {
		result.SafetyOK = false
		result.SafetyError = err.Error()
		return result, nil // Return the result so the LLM can reason about it
	}
	result.SafetyOK = true

	// Sandbox test with no arguments (smoke test).
	testResult, err := c.sandbox.Run(ctx, script, []string{"--help"})
	if err != nil {
		c.log.Warn("sandbox test failed", zap.Error(err))
	}
	result.TestResult = testResult

	return result, nil
}

// Persist saves a drafted tool to disk after user confirmation.
func (c *Creator) Persist(meta ToolMeta, script string) error {
	return c.registry.Save(meta, script)
}

// List returns all persisted auto-generated tools.
func (c *Creator) List() ([]ToolMeta, error) {
	return c.registry.List()
}

// Registry returns the underlying registry.
func (c *Creator) Registry() *Registry {
	return c.registry
}
