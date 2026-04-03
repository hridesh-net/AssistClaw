package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed templates/*.md
var embeddedTemplates embed.FS

// InitializeWorkspace creates the base directories (~/.assistclaw, tools, skills)
// and drops default assistclaw.yaml and workspace markdown templates if they don't already exist.
func InitializeWorkspace(configPath string) error {
	dir := filepath.Dir(configPath)

	// Create core directories
	dirs := []string{
		dir,
		filepath.Join(dir, "memory"),
		filepath.Join(dir, "skills"),
		filepath.Join(dir, "tools"),
		filepath.Join(dir, "policies"),
		filepath.Join(dir, "workspace", "public"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// Dump embedded markdown templates into the workspace (SOUL.md, IDENTITY.md, etc.)
	entries, err := embeddedTemplates.ReadDir("templates")
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			destPath := filepath.Join(dir, entry.Name())
			if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
				if data, readErr := embeddedTemplates.ReadFile(filepath.Join("templates", entry.Name())); readErr == nil {
					_ = os.WriteFile(destPath, data, 0o644)
				}
			}
		}
	}

	// We no longer write a default config.yaml here; the new interactive onboard wizard
	// will generate the custom config once the user inputs their preferences.

	return nil
}
