package kernel

import (
	"os"
	"path/filepath"

	"github.com/assistclaw/assistclaw/internal/skills"
)

// ExtractBundledSkills copies the repo's skills/ directory into destDir (the bundled dir).
// It searches for the skills directory relative to the binary, CWD, or common install paths.
func ExtractBundledSkills(destDir string) error {
	// Check if already populated (skip to avoid overwriting user edits)
	if info, err := os.ReadDir(destDir); err == nil && len(info) > 0 {
		return nil // already extracted
	}

	// Find the source skills directory
	src := ResolveBundledSkillsSrc()
	if src == "" {
		// Not available (e.g., installed without repo) — that's OK, marketplace will handle it
		return nil
	}

	return skills.CopyDir(src, destDir)
}

// ResolveBundledSkillsSrc locates the bundled skills/ directory relative to common install paths.
func ResolveBundledSkillsSrc() string {
	candidates := []string{}

	// 1. Relative to the running binary
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "skills"))
	}

	// 2. Relative to CWD (development mode)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "skills"))
	}

	// 3. Common install locations
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".assistclaw", "repo", "skills"))
	}
	candidates = append(candidates,
		"/usr/local/share/assistclaw/skills",
		"/opt/assistclaw/skills",
	)

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}
