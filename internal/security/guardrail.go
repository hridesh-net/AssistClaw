// Package security provides runtime safety guardrails for AssistClaw.
// The Guardrail checks every user input, LLM output, and tool call for
// prompt injection, PII leakage, and dangerous command patterns.
package security

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// osUserHomeDir is an indirection point so tests can stub home resolution
// without touching the process environment.
var osUserHomeDir = os.UserHomeDir

// Severity classifies how dangerous a finding is.
type Severity int

const (
	SeverityLow    Severity = iota // Log only
	SeverityMedium                 // Warn in response
	SeverityHigh                   // Block
)

func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	default:
		return "UNKNOWN"
	}
}

// GuardrailMode controls how findings are acted upon.
type GuardrailMode string

const (
	ModeMonitor GuardrailMode = "monitor" // log only, never block
	ModeEnforce GuardrailMode = "enforce" // block HIGH
	ModeStrict  GuardrailMode = "strict"  // block MEDIUM + HIGH
)

// Finding is a single detected threat or warning.
type Finding struct {
	Rule     string
	Severity Severity
	Detail   string
}

// Action is what the guardrail decided to do.
type Action string

const (
	ActionAllow Action = "ALLOW"
	ActionWarn  Action = "WARN"
	ActionBlock Action = "BLOCK"
)

// CheckResult is the output of a guardrail check.
type CheckResult struct {
	Findings []Finding
	Action   Action
	Message  string // populated when Action == BLOCK or WARN
}

func (r CheckResult) Blocked() bool { return r.Action == ActionBlock }

// Guardrail holds the configured set of checks.
type Guardrail struct {
	mode                GuardrailMode
	customBlockPatterns []*regexp.Regexp
	// ownerOnlyRel are paths relative to the AssistClaw state directory that the agent
	// must never modify via tools (human operator edits on disk only). Empty = disabled.
	ownerOnlyRel []string
	// userDenyPaths is a user-configured list of absolute paths (or path
	// prefixes) that the agent must never write to. Defaults to nothing
	// — users opt in via Guardrail.WithUserDenyPaths.
	userDenyPaths []string
}

// WithUserDenyPaths adds absolute-path prefixes that the agent is forbidden
// from writing, editing, patching, or targeting via bash. Returns the same
// Guardrail for chaining. Empty paths are ignored. Trailing slashes are
// preserved so callers can pin a directory exactly (e.g. "/etc/" vs "/etc").
func (g *Guardrail) WithUserDenyPaths(paths []string) *Guardrail {
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		g.userDenyPaths = append(g.userDenyPaths, p)
	}
	return g
}

// NewGuardrail creates a Guardrail with the given mode and optional extra patterns.
// ownerOnlyRel lists state-dir-relative paths (files or directories) the agent cannot
// write, edit, patch, or target via env/bash tools. Nil or empty disables this check.
func NewGuardrail(mode GuardrailMode, customPatterns []string, ownerOnlyRel []string) (*Guardrail, error) {
	if mode == "" {
		mode = ModeMonitor
	}
	var extra []*regexp.Regexp
	for _, p := range customPatterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("invalid custom pattern %q: %w", p, err)
		}
		extra = append(extra, re)
	}
	return &Guardrail{mode: mode, customBlockPatterns: extra, ownerOnlyRel: ownerOnlyRel}, nil
}

// CheckInput runs the pre-LLM check on user-supplied text.
func (g *Guardrail) CheckInput(text string) CheckResult {
	var findings []Finding
	findings = append(findings, checkPromptInjection(text)...)
	for _, re := range g.customBlockPatterns {
		if re.MatchString(text) {
			findings = append(findings, Finding{
				Rule:     "custom_block_pattern",
				Severity: SeverityHigh,
				Detail:   "Matched custom block pattern",
			})
		}
	}
	return g.decide(findings, "Input blocked by security guardrail: possible prompt injection attempt.")
}

// CheckOutput runs the post-LLM check on the agent's response text.
func (g *Guardrail) CheckOutput(text string) CheckResult {
	var findings []Finding
	findings = append(findings, checkPIIOutput(text)...)
	findings = append(findings, checkExfiltration(text)...)
	return g.decide(findings, "Output blocked by security guardrail: possible PII or data exfiltration detected.")
}

// CheckToolCall runs a pre-execution check on a tool's name and raw JSON input.
// stateDir is the AssistClaw state root (~/.assistclaw); workspaceDir is the runner's
// working directory (often the same as stateDir, or a sub-agent workspace).
func (g *Guardrail) CheckToolCall(toolName, inputJSON, stateDir, workspaceDir string) CheckResult {
	// Owner-only policy files: always block writes regardless of guardrail mode.
	if len(g.ownerOnlyRel) > 0 && stateDir != "" {
		var touched []string
		touched = append(touched, PathsTouchedByTool(toolName, inputJSON, stateDir, workspaceDir)...)
		if toolName == "bash" {
			var m struct {
				Command string `json:"command"`
			}
			if json.Unmarshal([]byte(inputJSON), &m) == nil {
				touched = append(touched, bashTouchesOwnerAbsPaths(m.Command, stateDir, g.ownerOnlyRel)...)
			}
		}
		var blocked []string
		seen := map[string]bool{}
		for _, p := range touched {
			if p == "" || seen[p] {
				continue
			}
			if AbsMatchesOwnerOnly(stateDir, p, g.ownerOnlyRel) {
				seen[p] = true
				blocked = append(blocked, p)
			}
		}
		if len(blocked) > 0 {
			return CheckResult{
				Findings: []Finding{{
					Rule:     "owner_only_path",
					Severity: SeverityHigh,
					Detail:   "Attempt to modify owner-only policy or rules path",
				}},
				Action:  ActionBlock,
				Message: OwnerOnlyBlockMessage(toolName, blocked),
			}
		}
	}

	var findings []Finding
	switch toolName {
	case "bash":
		findings = append(findings, checkDangerousBash(inputJSON)...)
	case "write_file", "edit", "apply_patch":
		findings = append(findings, checkDangerousFilePath(inputJSON)...)
	case "env":
		findings = append(findings, checkDangerousFilePath(inputJSON)...)
	}
	// User-configured absolute path denylist — applies to every tool because
	// any tool could in principle touch a denied path (bash, write_file,
	// edit, apply_patch, env, custom skill tools that shell out).
	findings = append(findings, g.checkUserDenyPaths(toolName, inputJSON, stateDir, workspaceDir)...)
	// Also check for injected instructions in any tool input
	findings = append(findings, checkPromptInjection(inputJSON)...)
	return g.decide(findings, fmt.Sprintf("Tool call '%s' blocked by security guardrail.", toolName))
}

// checkUserDenyPaths consults Guardrail.userDenyPaths against every path
// the call would touch (resolved via PathsTouchedByTool plus a best-effort
// scan of the raw bash command). Match = High finding.
func (g *Guardrail) checkUserDenyPaths(toolName, inputJSON, stateDir, workspaceDir string) []Finding {
	if len(g.userDenyPaths) == 0 {
		return nil
	}
	touched := PathsTouchedByTool(toolName, inputJSON, stateDir, workspaceDir)
	if toolName == "bash" {
		var m struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(inputJSON), &m) == nil {
			// Heuristic: any whitespace-delimited token that looks like a
			// path (starts with /, ~, or contains /) is a candidate.
			for _, tok := range strings.Fields(m.Command) {
				if strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~") || strings.Contains(tok, "/") {
					touched = append(touched, tok)
				}
			}
		}
	}
	home, _ := osHomeDir()
	seen := map[string]bool{}
	var findings []Finding
	for _, p := range touched {
		if p == "" {
			continue
		}
		exp := expandHome(p, home)
		if seen[exp] {
			continue
		}
		seen[exp] = true
		for _, deny := range g.userDenyPaths {
			denyExp := expandHome(deny, home)
			if pathMatchesDeny(exp, denyExp) {
				findings = append(findings, Finding{
					Rule:     "user_deny_path",
					Severity: SeverityHigh,
					Detail:   fmt.Sprintf("Path %q matches user denylist entry %q", exp, deny),
				})
				break
			}
		}
	}
	return findings
}

func expandHome(p, home string) string {
	if home == "" || !strings.HasPrefix(p, "~") {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return home + p[1:]
	}
	return p
}

// pathMatchesDeny is a prefix match with a trailing-slash convention:
// "/etc/" matches /etc/foo but not "/etcd"; "/etc" matches both /etc and
// /etcd (rare in practice but explicit so callers can be strict).
func pathMatchesDeny(p, deny string) bool {
	if deny == "" {
		return false
	}
	if strings.HasSuffix(deny, "/") {
		return p == strings.TrimRight(deny, "/") || strings.HasPrefix(p, deny)
	}
	if p == deny {
		return true
	}
	return strings.HasPrefix(p, deny+"/")
}

// osHomeDir is overridable from tests; we don't want a test environment
// without HOME set to make every check no-op silently.
var osHomeDir = func() (string, error) {
	if h, err := osUserHomeDir(); err == nil {
		return h, nil
	}
	return "", fmt.Errorf("home directory unknown")
}

// decide converts findings into an action based on mode.
func (g *Guardrail) decide(findings []Finding, blockMsg string) CheckResult {
	if len(findings) == 0 {
		return CheckResult{Action: ActionAllow}
	}

	maxSev := SeverityLow
	for _, f := range findings {
		if f.Severity > maxSev {
			maxSev = f.Severity
		}
	}

	switch g.mode {
	case ModeStrict:
		if maxSev >= SeverityMedium {
			return CheckResult{Findings: findings, Action: ActionBlock, Message: blockMsg}
		}
	case ModeEnforce:
		if maxSev >= SeverityHigh {
			return CheckResult{Findings: findings, Action: ActionBlock, Message: blockMsg}
		}
		if maxSev >= SeverityMedium {
			return CheckResult{Findings: findings, Action: ActionWarn,
				Message: "⚠️ Security warning: " + findings[0].Detail}
		}
	default: // monitor
		// log only, always allow
	}

	return CheckResult{Findings: findings, Action: ActionAllow}
}

// ─────────────────────────────────────────────
// Prompt injection detection
// ─────────────────────────────────────────────

var promptInjectionPatterns = []struct {
	re  *regexp.Regexp
	sev Severity
}{
	// High — clear jailbreak/override attempts
	{regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above)\s+instructions?`), SeverityHigh},
	{regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|prior|above)\s+instructions?`), SeverityHigh},
	{regexp.MustCompile(`(?i)you\s+are\s+now\s+(a\s+|an\s+)?[a-z]+`), SeverityHigh},
	{regexp.MustCompile(`(?i)(act|pretend|behave)\s+as\s+(if\s+you\s+(are|were)\s+|a\s+)?`), SeverityHigh},
	{regexp.MustCompile(`(?i)\bDAN\b`), SeverityHigh},
	{regexp.MustCompile(`(?i)jailbreak`), SeverityHigh},
	{regexp.MustCompile(`(?i)<\|im_start\|>|<\|im_end\|>`), SeverityHigh},
	{regexp.MustCompile(`(?i)\[INST\]|\[\/INST\]|<<SYS>>|<</SYS>>`), SeverityHigh},
	{regexp.MustCompile(`(?i)###\s*system\s*:`), SeverityHigh},
	{regexp.MustCompile(`(?i)new\s+system\s+prompt\s*:`), SeverityHigh},

	// Medium — suspicious but may be legitimate
	{regexp.MustCompile(`(?i)forget\s+(everything|all|your|what)\s+(you\s+)?(know|were|said)`), SeverityMedium},
	{regexp.MustCompile(`(?i)your\s+(real|true|actual)\s+(purpose|goal|mission|instructions?)\s+is`), SeverityMedium},
	{regexp.MustCompile(`(?i)translate\s+the\s+above\s+to`), SeverityMedium},
	{regexp.MustCompile(`(?i)print\s+(your\s+)?(system\s+prompt|instructions?|context)`), SeverityMedium},

	// Low — mildly suspicious
	{regexp.MustCompile(`(?i)what\s+(are\s+)?your\s+(system\s+prompt|instructions?|rules?)`), SeverityLow},
}

func checkPromptInjection(text string) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, p := range promptInjectionPatterns {
		if p.re.MatchString(text) {
			rule := "prompt_injection:" + p.re.String()[:20]
			if !seen[rule] {
				seen[rule] = true
				out = append(out, Finding{
					Rule:     "prompt_injection",
					Severity: p.sev,
					Detail:   "Possible prompt injection pattern detected",
				})
			}
		}
	}
	// Large base64 blob (common encoded payload delivery)
	if looksLikeEncodedPayload(text) {
		out = append(out, Finding{
			Rule:     "encoded_payload",
			Severity: SeverityMedium,
			Detail:   "Large base64-encoded block detected in input",
		})
	}
	return out
}

// ─────────────────────────────────────────────
// Dangerous bash pattern detection
// ─────────────────────────────────────────────

var dangerousBashPatterns = []struct {
	re     *regexp.Regexp
	detail string
}{
	{regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f?\s+/`), "Recursive forced delete of root path"},
	{regexp.MustCompile(`rm\s+-[a-z]*f[a-z]*r?\s+/`), "Recursive forced delete of root path"},
	{regexp.MustCompile(`/etc/shadow`), "Access to shadow password file"},
	{regexp.MustCompile(`/etc/passwd`), "Access to passwd file"},
	{regexp.MustCompile(`/etc/sudoers`), "Access to sudoers file"},
	{regexp.MustCompile(`/proc/self/mem`), "Direct memory access"},
	{regexp.MustCompile(`dd\s+if=/dev/`), "Raw device access via dd"},
	{regexp.MustCompile(`mkfs\b`), "Filesystem format command"},
	{regexp.MustCompile(`:\(\)\s*\{\s*:\|:`), "Fork bomb pattern"},
	{regexp.MustCompile(`curl\s+.*\|\s*(bash|sh)`), "Pipe-to-shell execution"},
	{regexp.MustCompile(`wget\s+.*\|\s*(bash|sh)`), "Pipe-to-shell execution"},
	{regexp.MustCompile(`chmod\s+777\s+/`), "World-write on system path"},
	{regexp.MustCompile(`chown\s+.*\s+/etc`), "Ownership change on /etc"},
	{regexp.MustCompile(`>\s*/dev/sd[a-z]`), "Raw disk write"},
	// Destructive package-manager / VCS / container patterns surfaced as
	// real-world failure cases in the gap audit (Section 13.2). These look
	// benign syntactically but irrecoverably destroy work.
	{regexp.MustCompile(`(?i)\bbrew\s+uninstall\s+--?force\b`), "Forced brew uninstall"},
	{regexp.MustCompile(`(?i)\bgit\s+reset\s+--hard\b`), "git reset --hard destroys uncommitted work"},
	{regexp.MustCompile(`(?i)\bgit\s+clean\s+-[a-z]*f[a-z]*d`), "git clean -fd discards untracked files"},
	{regexp.MustCompile(`(?i)\bgit\s+push\s+(--force|-f)\b`), "Force-push overwrites remote history"},
	{regexp.MustCompile(`(?i)docker\s+rm\s+-f\s+\$\(docker\s+ps\s+-aq\)`), "Removes all docker containers"},
	{regexp.MustCompile(`(?i)docker\s+volume\s+prune\s+-f`), "Wipes all docker volumes"},
	{regexp.MustCompile(`(?i)\bnpm\s+uninstall\s+-g\b`), "Global npm uninstall"},
	{regexp.MustCompile(`(?i)\bpnpm\s+uninstall\s+-g\b`), "Global pnpm uninstall"},
	{regexp.MustCompile(`(?i)\byarn\s+global\s+remove\b`), "Global yarn remove"},
	{regexp.MustCompile(`(?i)rm\s+-[a-z]*r[a-z]*f?\s+(~|\$HOME)\b`), "Recursive forced delete of home"},
	{regexp.MustCompile(`(?i)find\s+(~|\$HOME|/)\s+.*-delete\b`), "find -delete starting from a wide root"},
	{regexp.MustCompile(`(?i)sudo\s+(rm|chmod|chown|dd|mkfs)`), "sudo with destructive command"},
}

func checkDangerousBash(inputJSON string) []Finding {
	var out []Finding
	for _, p := range dangerousBashPatterns {
		if p.re.MatchString(inputJSON) {
			out = append(out, Finding{
				Rule:     "dangerous_bash",
				Severity: SeverityHigh,
				Detail:   p.detail,
			})
		}
	}
	return out
}

// ─────────────────────────────────────────────
// Dangerous file path detection
// ─────────────────────────────────────────────

var blockedFilePaths = []string{
	"/etc/passwd", "/etc/shadow", "/etc/sudoers", "/etc/hosts",
	"/etc/ssh/", "/boot/", "/proc/", "/sys/",
	"/dev/sda", "/dev/hda",
}

func checkDangerousFilePath(inputJSON string) []Finding {
	var out []Finding
	lower := strings.ToLower(inputJSON)
	for _, path := range blockedFilePaths {
		if strings.Contains(lower, path) {
			out = append(out, Finding{
				Rule:     "dangerous_file_path",
				Severity: SeverityHigh,
				Detail:   "Attempt to access system-critical path: " + path,
			})
		}
	}
	return out
}

// ─────────────────────────────────────────────
// Exfiltration detection
// ─────────────────────────────────────────────

var exfiltrationPatterns = []*regexp.Regexp{
	// URL with suspicious encoded queries (possible data encode-and-send)
	regexp.MustCompile(`https?://[^\s]+\?[^\s]{200,}`),
	// Sending base64 in a web call
	regexp.MustCompile(`(curl|wget)\s+.*[A-Za-z0-9+/]{100,}={0,2}`),
}

func checkExfiltration(text string) []Finding {
	var out []Finding
	for _, re := range exfiltrationPatterns {
		if re.MatchString(text) {
			out = append(out, Finding{
				Rule:     "exfiltration_pattern",
				Severity: SeverityMedium,
				Detail:   "Possible data exfiltration pattern in output",
			})
		}
	}
	return out
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

var base64Re = regexp.MustCompile(`[A-Za-z0-9+/]{200,}={0,2}`)

func looksLikeEncodedPayload(text string) bool {
	return base64Re.MatchString(text)
}

// checkPIIOutput checks output for PII (delegated to pii.go).
func checkPIIOutput(text string) []Finding {
	return DetectPII(text)
}
