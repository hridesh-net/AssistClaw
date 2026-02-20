package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// InitializeWorkspace creates the base directories (~/.assistclaw, tools, skills)
// and drops a default assistclaw.yaml if it doesn't already exist.
func InitializeWorkspace(configPath string) error {
	dir := filepath.Dir(configPath)

	// Create core directories
	dirs := []string{
		dir,
		filepath.Join(dir, "memory"),
		filepath.Join(dir, "skills"),
		filepath.Join(dir, "tools"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// Dump a default YAML config if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		const defaultConfig = `# AssistClaw default configuration
version: 1

routing:
  default: "anthropic/claude-3-5-haiku-20241022"
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0o600); err != nil {
			return fmt.Errorf("failed to write default config: %w", err)
		}
	}

	return nil
}
