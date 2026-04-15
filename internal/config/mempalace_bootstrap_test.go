package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMemPalaceBootstrapConfig_stateDirAndPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := MemPalaceBootstrapConfig(root)
	if cfg.StateDir != root {
		t.Fatalf("StateDir: got %q want %q", cfg.StateDir, root)
	}
	ep := filepath.Join(root, "memory", "episodic.db")
	if cfg.Memory.EpisodicDBPath != ep {
		t.Fatalf("EpisodicDBPath: got %q want %q", cfg.Memory.EpisodicDBPath, ep)
	}
	if !strings.Contains(cfg.Memory.SemanticDBPath, root) {
		t.Fatalf("SemanticDBPath %q should be under state dir", cfg.Memory.SemanticDBPath)
	}
	ci := filepath.Join(root, "localintel")
	if cfg.Agent.LocalIntel.CacheDir != ci {
		t.Fatalf("LocalIntel.CacheDir: got %q want %q", cfg.Agent.LocalIntel.CacheDir, ci)
	}
}
