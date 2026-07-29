package proactive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// RuleWatcher monitors a rules YAML file and hot-reloads the engine on changes.
type RuleWatcher struct {
	path   string
	engine *Engine
	log    *zap.Logger
}

// NewRuleWatcher creates a file watcher for the given rules path.
func NewRuleWatcher(path string, engine *Engine, log *zap.Logger) *RuleWatcher {
	if log == nil {
		log = zap.NewNop()
	}
	return &RuleWatcher{path: path, engine: engine, log: log}
}

// Start blocks, watching the file for changes until ctx is cancelled.
func (w *RuleWatcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify new watcher: %w", err)
	}
	defer watcher.Close()

	dir := filepath.Dir(w.path)
	if dir == "" {
		dir = "."
	}
	if err := watcher.Add(dir); err != nil {
		return fmt.Errorf("fsnotify watch dir: %w", err)
	}

	// Initial load
	w.reload()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("fsnotify events channel closed")
			}
			if event.Name == w.path && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
				w.reload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("fsnotify errors channel closed")
			}
			w.log.Warn("fsnotify error", zap.Error(err))
		}
	}
}

func (w *RuleWatcher) reload() {
	if _, err := os.Stat(w.path); os.IsNotExist(err) {
		w.log.Debug("rules file does not exist, clearing rules", zap.String("path", w.path))
		if err := w.engine.SetRules(nil); err != nil {
			w.log.Warn("failed to clear rules", zap.Error(err))
		}
		return
	}

	rules, err := LoadRulesFromYAML(w.path, w.engine.triggers, w.engine.actions, w.engine.notifiers)
	if err != nil {
		w.log.Warn("rules reload failed, keeping old rules", zap.String("path", w.path), zap.Error(err))
		return
	}
	if err := w.engine.SetRules(rules); err != nil {
		w.log.Warn("failed to apply rules", zap.Error(err))
		return
	}
	w.log.Info("rules reloaded", zap.String("path", w.path), zap.Int("count", len(rules)))
}
