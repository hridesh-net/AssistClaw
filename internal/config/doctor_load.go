package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DoctorIssue is a config-level finding for assistclaw doctor (parse, validate, deprecation).
// Line and Column are 1-based when known (YAML parse); 0 means unknown.
type DoctorIssue struct {
	Severity string // "error" | "warn"
	Message  string
	File     string
	Line     int
	Column   int
}

// deprecatedYAMLKey maps dotted paths (e.g. "routing.primary") to replacement hints.
var deprecatedYAMLKey = map[string]string{
	"routing.primary": "routing.default",
}

// LoadForDoctor reads and validates a config file like [Load], but returns structured issues
// (validation errors, deprecated keys, optional version hint) instead of failing the whole load.
// YAML syntax errors still return a non-nil error and no config.
// Semantic validation errors are returned as DoctorIssue with Severity "error" and cfg may still be non-nil.
func LoadForDoctor(path string) (*Config, []DoctorIssue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	expanded := os.ExpandEnv(string(data))

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(expanded), &root); err != nil {
		return nil, nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		return nil, nil, fmt.Errorf("config: decode %s: %w", path, err)
	}

	applyDefaults(&cfg)

	var issues []DoctorIssue
	issues = append(issues, collectDeprecatedYAMLKeys(&root, path)...)

	if cfg.Version == 0 {
		issues = append(issues, DoctorIssue{
			Severity: "warn",
			Message:  "top-level `version` is unset; set `version: 1` so future schema migrations are explicit",
			File:     path,
			Line:     0,
			Column:   0,
		})
	}

	if err := validate(&cfg); err != nil {
		issues = append(issues, validationIssuesFromError(path, err)...)
	}

	return &cfg, issues, nil
}

func validationIssuesFromError(path string, err error) []DoctorIssue {
	s := err.Error()
	const prefix = "config validation errors:\n"
	if !strings.HasPrefix(s, prefix) {
		return []DoctorIssue{{
			Severity: "error",
			Message:  strings.TrimSpace(s),
			File:     path,
		}}
	}
	body := strings.TrimPrefix(s, prefix)
	parts := strings.Split(body, "\n  - ")
	out := make([]DoctorIssue, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "-")
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, DoctorIssue{
			Severity: "error",
			Message:  p,
			File:     path,
		})
	}
	return out
}

func collectDeprecatedYAMLKeys(root *yaml.Node, file string) []DoctorIssue {
	if root == nil {
		return nil
	}
	var out []DoctorIssue
	var walk func(n *yaml.Node, path []string)
	walk = func(n *yaml.Node, path []string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c, path)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				kn := n.Content[i]
				vn := n.Content[i+1]
				key := kn.Value
				newPath := append(path, key)
				dotted := strings.Join(newPath, ".")
				if repl, ok := deprecatedYAMLKey[dotted]; ok {
					out = append(out, DoctorIssue{
						Severity: "warn",
						Message:  fmt.Sprintf("deprecated key %q — use %s", dotted, repl),
						File:     file,
						Line:     kn.Line,
						Column:   kn.Column,
					})
				}
				walk(vn, newPath)
			}
		case yaml.SequenceNode:
			for idx, c := range n.Content {
				walk(c, append(path, fmt.Sprintf("[%d]", idx)))
			}
		case yaml.ScalarNode, yaml.AliasNode:
			return
		default:
			for _, c := range n.Content {
				walk(c, path)
			}
		}
	}
	walk(root, nil)
	return out
}
