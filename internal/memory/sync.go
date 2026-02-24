package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/embeddings"
)

// SyncFiles scans the state directory for .md files and indexes them.
func (m *Manager) SyncFiles(ctx context.Context, registry *embeddings.Registry, workspaceDir string) error {
	// 1. List all memory files (.md)
	files, err := listMemoryFiles(workspaceDir)
	if err != nil {
		return fmt.Errorf("list memory files: %w", err)
	}

	// 2. Index each file if changed
	for _, absPath := range files {
		relPath, _ := filepath.Rel(workspaceDir, absPath)
		if err := m.syncFile(ctx, registry, workspaceDir, absPath, relPath); err != nil {
			fmt.Printf("[memory] error syncing %s: %v\n", relPath, err)
		}
	}

	// 3. Cleanup stale entries (documents whose source file is gone)
	// TODO: implement cleanup logic

	return nil
}

func (m *Manager) syncFile(ctx context.Context, registry *embeddings.Registry, workspaceDir, absPath, relPath string) error {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(content))

	// Check if the file (source) already has this hash in the DB
	var dbHash string
	err = m.Semantic.db.QueryRowContext(ctx, "SELECT hash FROM documents WHERE source = ? LIMIT 1", relPath).Scan(&dbHash)
	if err == nil && dbHash == hash {
		// No change, skip
		return nil
	}

	fmt.Printf("[memory] indexing %s (hash: %s...)\n", relPath, hash[:8])

	// Clear old chunks for this file
	if err := m.Semantic.DeleteBySource(ctx, relPath); err != nil {
		return err
	}

	// Chunk the file
	chunks := ChunkMarkdown(string(content), 512, 64)
	if len(chunks) == 0 {
		return nil
	}

	// Prepare texts for embedding
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	// Get embeddings for all chunks in a batch
	e, ok := registry.Default()
	if !ok {
		return fmt.Errorf("no embedding provider available")
	}

	resp, err := e.Embed(ctx, &embeddings.EmbedRequest{
		Model:     e.DefaultModel(),
		Texts:     texts,
		InputType: embeddings.InputTypeDocument,
	})
	if err != nil {
		return fmt.Errorf("embed chunks for %s: %w", relPath, err)
	}

	if len(resp.Embeddings) != len(chunks) {
		return fmt.Errorf("dimension mismatch: got %d embeddings for %d chunks", len(resp.Embeddings), len(chunks))
	}

	// Index chunks
	for i, c := range chunks {
		doc := Document{
			ID:        fmt.Sprintf("%s#L%d-%d", relPath, c.StartLine, c.EndLine),
			Source:    relPath,
			Content:   c.Content,
			Hash:      hash, // Use the full file hash for all chunks
			Embedding: resp.Embeddings[i],
			CreatedAt: time.Now(),
		}
		if err := m.Semantic.Index(ctx, doc); err != nil {
			return fmt.Errorf("index chunk %d of %s: %w", i, relPath, err)
		}
	}

	return nil
}

func listMemoryFiles(workspaceDir string) ([]string, error) {
	var files []string

	// OpenClaw specific logic: MEMORY.md and memory/*.md
	memoryFile := filepath.Join(workspaceDir, "MEMORY.md")
	if _, err := os.Stat(memoryFile); err == nil {
		files = append(files, memoryFile)
	}

	memoryDir := filepath.Join(workspaceDir, "memory")
	if stat, err := os.Stat(memoryDir); err == nil && stat.IsDir() {
		filepath.Walk(memoryDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
				files = append(files, path)
			}
			return nil
		})
	}

	return files, nil
}
