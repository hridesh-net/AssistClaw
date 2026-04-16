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
	"time"
)

// DefaultGGUFURL is the default model download used by `assistclaw local-intel setup`
// when no explicit URL/env override is provided. It points to AssistClaw's own release assets.
const DefaultGGUFURL = "https://github.com/hridesh-net/AssistClaw/releases/latest/download/gemma-4-e2b-it.gguf"

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
	if tok := strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
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
	var progress *downloadProgress
	if opt.Progress != nil {
		progress = newDownloadProgress(opt.Progress, resp.ContentLength)
		src = io.TeeReader(src, progress)
	}
	n, copyErr := io.Copy(out, src)
	if progress != nil {
		progress.finish()
	}
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

type downloadProgress struct {
	w           io.Writer
	total       int64
	downloaded  int64
	startedAt   time.Time
	lastPrinted time.Time
	lastWidth   int
}

func newDownloadProgress(w io.Writer, total int64) *downloadProgress {
	now := time.Now()
	return &downloadProgress{
		w:           w,
		total:       total,
		startedAt:   now,
		lastPrinted: now,
	}
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	n := len(b)
	p.downloaded += int64(n)
	now := time.Now()
	if now.Sub(p.lastPrinted) >= 500*time.Millisecond {
		p.print(false)
		p.lastPrinted = now
	}
	return n, nil
}

func (p *downloadProgress) finish() {
	p.print(true)
}

func (p *downloadProgress) print(done bool) {
	secs := time.Since(p.startedAt).Seconds()
	speed := 0.0
	if secs > 0 {
		speed = float64(p.downloaded) / (1024 * 1024) / secs
	}
	if p.total > 0 {
		pct := (float64(p.downloaded) / float64(p.total)) * 100
		if pct > 100 {
			pct = 100
		}
		line := fmt.Sprintf(
			"GGUF [%s] %3.0f%% %.0f/%.0fMB %.1fMB/s",
			renderProgressBar(pct, 16),
			pct,
			float64(p.downloaded)/(1024*1024),
			float64(p.total)/(1024*1024),
			speed,
		)
		p.writeLine(line)
	} else {
		line := fmt.Sprintf(
			"GGUF %.0fMB %.1fMB/s",
			float64(p.downloaded)/(1024*1024),
			speed,
		)
		p.writeLine(line)
	}
	if done {
		_, _ = fmt.Fprint(p.w, "\n")
	}
}

func (p *downloadProgress) writeLine(line string) {
	padding := ""
	if extra := p.lastWidth - len(line); extra > 0 {
		padding = strings.Repeat(" ", extra)
	}
	_, _ = fmt.Fprintf(p.w, "\r%s%s", line, padding)
	p.lastWidth = len(line)
}

func renderProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 20
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int((percent / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}
