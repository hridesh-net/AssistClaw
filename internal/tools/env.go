package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// EnvTool reads and writes environment variables and .env files.
// Critical for project setup, checking secrets, and managing configuration.
type EnvTool struct{}

func (EnvTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "env",
		Description: `Read or write environment variables and .env files.
Commands:
  get      — read the value of a specific OS environment variable
  list     — list all environment variables (or filter by prefix)
  set      — set an environment variable for the current process
  read_file — read a .env file and return key=value pairs
  write_file — write or update key=value pairs in a .env file`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "One of: get, list, set, read_file, write_file",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Variable name (for get/set) or prefix filter (for list)",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Value to set (for set/write_file)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Path to .env file (for read_file/write_file, default .env)",
				},
				"pairs": map[string]any{
					"type":        "object",
					"description": "Map of key→value to write (for write_file)",
				},
			},
			Required: []string{"command"},
		},
	}
}

func (EnvTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command string            `json:"command"`
		Key     string            `json:"key"`
		Value   string            `json:"value"`
		Path    string            `json:"path"`
		Pairs   map[string]string `json:"pairs"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	switch strings.ToLower(args.Command) {
	case "get":
		if args.Key == "" {
			return "env get: 'key' is required", nil
		}
		val, ok := os.LookupEnv(args.Key)
		if !ok {
			return fmt.Sprintf("Environment variable %q is not set.", args.Key), nil
		}
		return fmt.Sprintf("%s=%s", args.Key, val), nil

	case "list":
		envs := os.Environ()
		prefix := strings.ToUpper(args.Key)
		var sb strings.Builder
		count := 0
		for _, e := range envs {
			if prefix == "" || strings.HasPrefix(strings.ToUpper(e), prefix) {
				// Hide secrets in values
				sb.WriteString(maskSecret(e) + "\n")
				count++
			}
		}
		if count == 0 {
			if prefix != "" {
				return fmt.Sprintf("No environment variables matching prefix %q.", args.Key), nil
			}
			return "No environment variables found.", nil
		}
		return fmt.Sprintf("=== Environment Variables (%d) ===\n%s", count, sb.String()), nil

	case "set":
		if args.Key == "" {
			return "env set: 'key' is required", nil
		}
		if err := os.Setenv(args.Key, args.Value); err != nil {
			return fmt.Sprintf("env set failed: %v", err), nil
		}
		return fmt.Sprintf("✔ %s=%s", args.Key, args.Value), nil

	case "read_file":
		path := envFilePath(args.Path)
		pairs, err := readDotEnv(path)
		if err != nil {
			return fmt.Sprintf("env read_file: %v", err), nil
		}
		if len(pairs) == 0 {
			return fmt.Sprintf("File %s is empty.", path), nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("=== %s (%d keys) ===\n", filepath.Base(path), len(pairs)))
		for k, v := range pairs {
			sb.WriteString(fmt.Sprintf("%s=%s\n", k, maskValue(k, v)))
		}
		return sb.String(), nil

	case "write_file":
		path := envFilePath(args.Path)
		if args.Pairs == nil && args.Key != "" {
			// Single key=value mode
			args.Pairs = map[string]string{args.Key: args.Value}
		}
		if len(args.Pairs) == 0 {
			return "env write_file: provide 'pairs' (object of key→value) or 'key'+'value'", nil
		}
		if err := writeDotEnv(path, args.Pairs); err != nil {
			return fmt.Sprintf("env write_file: %v", err), nil
		}
		keys := make([]string, 0, len(args.Pairs))
		for k := range args.Pairs {
			keys = append(keys, k)
		}
		return fmt.Sprintf("✔ Updated %s: %s", path, strings.Join(keys, ", ")), nil

	default:
		return fmt.Sprintf("Unknown env command %q. Use: get, list, set, read_file, write_file", args.Command), nil
	}
}

func envFilePath(p string) string {
	if p == "" {
		return ".env"
	}
	return p
}

// readDotEnv parses a .env file into a key→value map.
func readDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pairs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		pairs[key] = val
	}
	return pairs, scanner.Err()
}

// writeDotEnv updates or creates entries in a .env file, preserving comments.
func writeDotEnv(path string, updates map[string]string) error {
	existing, _ := readDotEnv(path)
	for k, v := range updates {
		existing[k] = v
	}

	// Read current file content to preserve comments/order, then update in-place
	var lines []string
	updated := make(map[string]bool)

	f, err := os.Open(path)
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}
			idx := strings.IndexByte(trimmed, '=')
			if idx < 0 {
				lines = append(lines, line)
				continue
			}
			key := strings.TrimSpace(trimmed[:idx])
			if newVal, ok := updates[key]; ok {
				lines = append(lines, key+"="+newVal)
				updated[key] = true
			} else {
				lines = append(lines, line)
			}
		}
		f.Close()
	}

	// Append new keys not already in the file
	for k, v := range updates {
		if !updated[k] {
			lines = append(lines, k+"="+v)
		}
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// maskSecret hides the value part of KEY=VALUE lines that look like secrets.
func maskSecret(kv string) string {
	idx := strings.IndexByte(kv, '=')
	if idx < 0 {
		return kv
	}
	key := kv[:idx]
	val := kv[idx+1:]
	return key + "=" + maskValue(key, val)
}

func maskValue(key, val string) string {
	keyUpper := strings.ToUpper(key)
	for _, sensitive := range []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "PASS", "PWD", "CREDENTIAL"} {
		if strings.Contains(keyUpper, sensitive) && len(val) > 4 {
			return val[:4] + strings.Repeat("*", len(val)-4)
		}
	}
	return val
}
