package localintel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultGGUFURL is the default model download used by `assistclaw local-intel setup`
// when no explicit URL/env override is provided.
const DefaultGGUFURL = "https://huggingface.co/bartowski/google_gemma-2-2b-it-GGUF/resolve/main/google_gemma-2-2b-it-Q4_K_M.gguf"

// DefaultGGUFPath returns the managed filesystem path used for downloaded GGUF weights.
func DefaultGGUFPath(stateDir string) string {
	return filepath.Join(stateDir, "models", "gemma-4-e2b-it.gguf")
}

// BootstrapOptions controls GGUF download/bootstrap behavior.
type BootstrapOptions struct {
	StateDir   string
	GGUFPath   string
	URL        string
	SHA256     string // optional lowercase/uppercase hex digest
	Force      bool
	Progress   io.Writer
	HTTPClient *http.Client
}

// BootstrapResult describes what happened during bootstrap.
type BootstrapResult struct {
	Path       string
	Downloaded bool
	Bytes      int64
}

// BootstrapGGUF ensures a GGUF exists on disk for local_intel. It reuses an existing file unless
// Force is set. When download is needed it fetches from URL (or env/default fallback) and optionally
// verifies SHA-256 when provided.
func BootstrapGGUF(ctx context.Context, opt BootstrapOptions) (BootstrapResult, error) {
	stateDir := strings.TrimSpace(opt.StateDir)
	if stateDir == "" {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: state_dir is empty")
	}
	dst := strings.TrimSpace(opt.GGUFPath)
	if dst == "" {
		dst = DefaultGGUFPath(stateDir)
	}
	if !opt.Force {
		if st, err := os.Stat(dst); err == nil && !st.IsDir() {
			return BootstrapResult{Path: dst, Downloaded: false, Bytes: st.Size()}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: mkdir for %q: %w", dst, err)
	}

	url := strings.TrimSpace(opt.URL)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF_URL"))
	}
	if url == "" {
		url = DefaultGGUFURL
	}
	sha := strings.ToLower(strings.TrimSpace(opt.SHA256))
	if sha == "" {
		sha = strings.ToLower(strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF_SHA256")))
	}

	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: GET %s: status %s", url, resp.Status)
	}

	tmp := dst + ".download"
	out, err := os.Create(tmp)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: create temp: %w", err)
	}

	hasher := sha256.New()
	src := io.TeeReader(resp.Body, hasher)
	if opt.Progress != nil {
		src = io.TeeReader(src, opt.Progress)
	}
	n, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: download/write: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: close temp: %w", closeErr)
	}

	gotSHA := strings.ToLower(hex.EncodeToString(hasher.Sum(nil)))
	if sha != "" && gotSHA != sha {
		_ = os.Remove(tmp)
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: sha256 mismatch: got %s want %s", gotSHA, sha)
	}

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return BootstrapResult{}, fmt.Errorf("localintel bootstrap: move into place: %w", err)
	}
	return BootstrapResult{Path: dst, Downloaded: true, Bytes: n}, nil
}
