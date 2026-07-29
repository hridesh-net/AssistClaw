package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/fsnotify/fsnotify"
)

// Watch scans the workspaceDir for changes and updates the memory index in real-time.
func (m *Manager) Watch(ctx context.Context, registry *embeddings.Registry, workspaceDir string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	// Initial sync
	if err := m.SyncFiles(ctx, registry, workspaceDir); err != nil {
		fmt.Printf("[memory] initial sync error: %v\n", err)
	}

	// Watch the root and the memory/ subdirectory
	if err := watcher.Add(workspaceDir); err != nil {
		return fmt.Errorf("watch root: %w", err)
	}
	memoryDir := filepath.Join(workspaceDir, "memory")
	_ = os.MkdirAll(memoryDir, 0755)
	if err := watcher.Add(memoryDir); err != nil {
		fmt.Printf("[memory] warning: could not watch memory directory: %v\n", err)
	}

	fmt.Printf("[memory] watching for changes in %s\n", workspaceDir)

	debouncer := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Only care about Markdown files
			if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
				continue
			}

			// Debounce write/rename/create events
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				if timer, ok := debouncer[event.Name]; ok {
					timer.Stop()
				}

				debouncer[event.Name] = time.AfterFunc(500*time.Millisecond, func() {
					delete(debouncer, event.Name)
					relPath, _ := filepath.Rel(workspaceDir, event.Name)
					if err := m.syncFile(ctx, registry, workspaceDir, event.Name, relPath); err != nil {
						fmt.Printf("[memory] error syncing changed file %s: %v\n", relPath, err)
					}
				})
			}

			// Delete event
			if event.Has(fsnotify.Remove) {
				relPath, _ := filepath.Rel(workspaceDir, event.Name)
				fmt.Printf("[memory] file removed: %s, clearing index\n", relPath)
				if err := m.Semantic.DeleteBySource(ctx, relPath); err != nil {
					fmt.Printf("[memory] error deleting source %s: %v\n", relPath, err)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("[memory] watcher error: %v\n", err)
		}
	}
}
