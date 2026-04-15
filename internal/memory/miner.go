package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/embeddings"
)

type MiningOptions struct {
	Mode           string
	Include        []string
	Exclude        []string
	MaxFilesPerRun int
	MaxFileSizeKB  int
	StatePath      string
	DryRun         bool
}

type MiningReport struct {
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Mode         string    `json:"mode"`
	Scanned      int       `json:"scanned"`
	Indexed      int       `json:"indexed"`
	Skipped      int       `json:"skipped"`
	Errors       int       `json:"errors"`
	LastError    string    `json:"last_error,omitempty"`
	SchemaVersion int      `json:"schema_version"`
}

func (m *Manager) Mine(ctx context.Context, registry *embeddings.Registry, workspaceDir string, opts MiningOptions) (MiningReport, error) {
	report := MiningReport{
		StartedAt:     time.Now(),
		Mode:          opts.Mode,
		SchemaVersion: 1,
	}
	if report.Mode == "" {
		report.Mode = "incremental"
	}
	if opts.MaxFilesPerRun <= 0 {
		opts.MaxFilesPerRun = 1000
	}
	if opts.MaxFileSizeKB <= 0 {
		opts.MaxFileSizeKB = 512
	}

	files, err := listMemoryFiles(workspaceDir)
	if err != nil {
		return report, err
	}
	for _, absPath := range files {
		if report.Scanned >= opts.MaxFilesPerRun {
			break
		}
		relPath, _ := filepath.Rel(workspaceDir, absPath)
		report.Scanned++
		if !matchesIncludeExclude(relPath, opts.Include, opts.Exclude) {
			report.Skipped++
			continue
		}
		if info, statErr := os.Stat(absPath); statErr == nil && info.Size() > int64(opts.MaxFileSizeKB)*1024 {
			report.Skipped++
			continue
		}
		if opts.DryRun {
			report.Indexed++
			continue
		}
		if syncErr := m.syncFile(ctx, registry, workspaceDir, absPath, relPath); syncErr != nil {
			report.Errors++
			report.LastError = syncErr.Error()
			continue
		}
		report.Indexed++
	}
	report.FinishedAt = time.Now()
	if opts.StatePath != "" && !opts.DryRun {
		_ = writeMiningState(opts.StatePath, report)
	}
	return report, nil
}

func ReadMiningState(path string) (MiningReport, error) {
	var report MiningReport
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	return report, nil
}

func writeMiningState(path string, report MiningReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func matchesIncludeExclude(path string, include, exclude []string) bool {
	if len(include) == 0 {
		include = []string{"MEMORY.md", "memory/*.md"}
	}
	path = filepath.ToSlash(path)
	matchedInclude := false
	for _, pattern := range include {
		if ok, _ := filepath.Match(pattern, path); ok {
			matchedInclude = true
			break
		}
		// For nested memory files where pattern uses * once.
		if strings.HasSuffix(pattern, "*.md") {
			prefix := strings.TrimSuffix(pattern, "*.md")
			if strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")) && strings.HasSuffix(path, ".md") {
				matchedInclude = true
				break
			}
		}
	}
	if !matchedInclude {
		return false
	}
	for _, pattern := range exclude {
		if ok, _ := filepath.Match(pattern, path); ok {
			return false
		}
	}
	return true
}

func (r MiningReport) PrettyString() string {
	return fmt.Sprintf("mode=%s scanned=%d indexed=%d skipped=%d errors=%d", r.Mode, r.Scanned, r.Indexed, r.Skipped, r.Errors)
}
