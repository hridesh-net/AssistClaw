package kernel

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/config"
)

// TestBuild_NoModelConfigured exercises the construction path end-to-end with a
// minimal config and no providers: Build should register (empty) provider and
// embedder sets, create the three memory tiers on disk, then fail cleanly at
// model resolution — returning the "no model configured" error, a nil App, and
// no panic. This is the runtime smoke for kernel.Build extracted from runAgent.
func TestBuild_NoModelConfigured(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir}
	cfg.Memory.WorkingTokenBudget = 8000
	cfg.Memory.EpisodicDBPath = filepath.Join(dir, "episodic.db")
	cfg.Memory.SemanticDBPath = filepath.Join(dir, "semantic.db")

	app, err := Build(context.Background(), cfg, zap.NewNop(), BuildOptions{})
	if err == nil {
		if app != nil {
			_ = app.Close()
		}
		t.Fatal("expected 'no model configured' error, got nil")
	}
	if !strings.Contains(err.Error(), "no model configured") {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != nil {
		t.Errorf("expected nil App on error (Build cleans up), got non-nil")
	}
}
