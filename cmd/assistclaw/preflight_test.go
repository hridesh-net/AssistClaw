package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/assistclaw/assistclaw/internal/observability/metrics"
	"go.uber.org/zap"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestRunPreflight_SkipDoesNotFail(t *testing.T) {
	var stderr bytes.Buffer
	gf := &globalFlags{configPath: "/nonexistent/path/assistclaw.yaml"}
	err := runPreflight(context.Background(), gf, preflightOpts{Skip: true}, zap.NewNop(), &stderr)
	if err != nil {
		t.Fatalf("expected nil: %v", err)
	}
	if !strings.Contains(stderr.String(), "WARNING: preflight skipped") {
		t.Fatalf("expected skip warning on stderr: %q", stderr.String())
	}
}

func TestRunPreflight_InvalidConfig(t *testing.T) {
	root := repoRoot(t)
	bad := filepath.Join(root, "internal", "config", "testdata", "doctor", "invalid_bad_gateway_port.yaml")
	metrics.Default().ResetForTests()

	gf := &globalFlags{configPath: bad}
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
	defer cancel()
	err := runPreflight(ctx, gf, preflightOpts{}, zap.NewNop(), &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("error: %v", err)
	}
	rendered := metrics.Default().RenderPrometheus()
	if !strings.Contains(rendered, "preflight_failures_total 1") {
		t.Fatalf("expected metric increment: %s", rendered)
	}
}

func TestRunPreflight_ValidMinimalFast(t *testing.T) {
	root := repoRoot(t)
	cfg := filepath.Join(root, "internal", "config", "testdata", "doctor", "valid_minimal.yaml")
	metrics.Default().ResetForTests()

	gf := &globalFlags{configPath: cfg, logLevel: "error"}
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
	defer cancel()
	err := runPreflight(ctx, gf, preflightOpts{}, zap.NewNop(), &stderr)
	if err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if strings.Contains(metrics.Default().RenderPrometheus(), "preflight_failures_total 1") {
		t.Fatal("unexpected preflight failure metric")
	}
}

func TestRunPreflight_ConfigReadError(t *testing.T) {
	tmp := t.TempDir()
	// Path exists but is a directory — read fails like a permission/config path mistake.
	badPath := filepath.Join(tmp, "assistclaw.yaml")
	if err := os.MkdirAll(badPath, 0o755); err != nil {
		t.Fatal(err)
	}
	metrics.Default().ResetForTests()

	gf := &globalFlags{configPath: badPath}
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
	defer cancel()
	err := runPreflight(ctx, gf, preflightOpts{}, zap.NewNop(), &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "preflight: config:") {
		t.Fatalf("got %v", err)
	}
}
