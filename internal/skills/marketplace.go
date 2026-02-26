package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultMarketplaceURL is the canonical index for AssistClaw skills.
const DefaultMarketplaceURL = "https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/skills/marketplace.json"

// MarketplaceEntry represents a skill available in the marketplace.
type MarketplaceEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Bundled     bool     `json:"bundled"` // true = ships with binary
	Tags        []string `json:"tags"`
	// For community skills:
	Repo    string `json:"repo,omitempty"`    // GitHub repo (e.g. "user/skill-name")
	URL     string `json:"url,omitempty"`     // Direct download URL
	Version string `json:"version,omitempty"` // Latest version tag
}

// MarketplaceIndex is the full marketplace catalog.
type MarketplaceIndex struct {
	Version string             `json:"version"`
	Updated string             `json:"updated"`
	Skills  []MarketplaceEntry `json:"skills"`
}

// Marketplace fetches and searches the skill catalog.
type Marketplace struct {
	IndexURL   string
	BundledDir string // path to bundled skills (embedded/extracted)
	CustomDir  string // path to user's custom skills
	client     *http.Client
}

// NewMarketplace creates a marketplace client.
func NewMarketplace(bundledDir, customDir string) *Marketplace {
	return &Marketplace{
		IndexURL:   DefaultMarketplaceURL,
		BundledDir: bundledDir,
		CustomDir:  customDir,
		client:     &http.Client{},
	}
}

// FetchIndex downloads the marketplace index from the network.
// Falls back to the bundled marketplace.json if offline.
func (m *Marketplace) FetchIndex(ctx context.Context) (*MarketplaceIndex, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.IndexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		// Fallback: read bundled marketplace.json
		return m.readBundledIndex()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return m.readBundledIndex()
	}

	var index MarketplaceIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, err
	}
	return &index, nil
}

// readBundledIndex reads the marketplace.json from the bundled skills dir.
func (m *Marketplace) readBundledIndex() (*MarketplaceIndex, error) {
	path := filepath.Join(m.BundledDir, "marketplace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("marketplace index unavailable (offline and no bundled index): %w", err)
	}
	var index MarketplaceIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// Search returns marketplace entries matching the query (name, description, or tag).
func (m *Marketplace) Search(ctx context.Context, query string) ([]MarketplaceEntry, error) {
	index, err := m.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var results []MarketplaceEntry
	for _, e := range index.Skills {
		if strings.Contains(strings.ToLower(e.Name), query) ||
			strings.Contains(strings.ToLower(e.Description), query) ||
			containsTag(e.Tags, query) {
			results = append(results, e)
		}
	}
	return results, nil
}

func containsTag(tags []string, q string) bool {
	for _, t := range tags {
		if strings.Contains(t, q) {
			return true
		}
	}
	return false
}

// Install copies a bundled skill or downloads a community skill to customDir.
// For bundled skills: copies from bundledDir.
// For community skills: clones/downloads from the entry's URL or Repo.
func (m *Marketplace) Install(ctx context.Context, nameOrURL string) (string, error) {
	// Normalize
	name := filepath.Base(nameOrURL)

	destDir := filepath.Join(m.CustomDir, name)
	if _, err := os.Stat(destDir); err == nil {
		return destDir, fmt.Errorf("skill %q is already installed at %s", name, destDir)
	}

	// Try bundled first
	bundledSrc := filepath.Join(m.BundledDir, name)
	if _, err := os.Stat(bundledSrc); err == nil {
		if err := CopyDir(bundledSrc, destDir); err != nil {
			return "", fmt.Errorf("failed to install bundled skill %q: %w", name, err)
		}
		return destDir, nil
	}

	// Try marketplace lookup
	index, err := m.FetchIndex(ctx)
	if err == nil {
		for _, e := range index.Skills {
			if e.Name == name {
				if downloadURL := resolveDownloadURL(e); downloadURL != "" {
					if err := downloadSkill(ctx, downloadURL, destDir); err != nil {
						return "", fmt.Errorf("failed to download skill %q: %w", name, err)
					}
					return destDir, nil
				}
			}
		}
	}

	// Try direct URL/GitHub
	if strings.HasPrefix(nameOrURL, "http") || strings.Contains(nameOrURL, "/") {
		url := resolveGitHubURL(nameOrURL)
		if err := downloadSkill(ctx, url, destDir); err != nil {
			return "", fmt.Errorf("failed to download from %s: %w", url, err)
		}
		return destDir, nil
	}

	return "", fmt.Errorf("skill %q not found in bundled skills or marketplace", name)
}

func resolveDownloadURL(e MarketplaceEntry) string {
	if e.URL != "" {
		return e.URL
	}
	if e.Repo != "" {
		return fmt.Sprintf("https://github.com/%s/archive/refs/heads/main.tar.gz", e.Repo)
	}
	return ""
}

func resolveGitHubURL(input string) string {
	if strings.HasPrefix(input, "http") {
		if !strings.HasSuffix(input, ".tar.gz") {
			return input + "/archive/refs/heads/main.tar.gz"
		}
		return input
	}
	// shorthand: user/repo
	return fmt.Sprintf("https://github.com/%s/archive/refs/heads/main.tar.gz", input)
}

// downloadSkill downloads a tar.gz archive and extracts the first directory into dest.
func downloadSkill(ctx context.Context, url string, destDir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to temp file, then extract
	tmp, err := os.CreateTemp("", "assistclaw-skill-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return err
	}
	tmp.Close()

	// Use tar to extract (available on all platforms via Go exec or stdlib)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	// Extract using go's archive/tar
	return extractTarGz(tmp.Name(), destDir)
}

// CopyDir recursively copies src to dst. Exported for use by the main package.
func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
