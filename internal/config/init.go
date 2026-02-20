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

	// We no longer write a default config.yaml here; the new interactive onboard wizard
	// will generate the custom config once the user inputs their preferences.

	return nil
}
