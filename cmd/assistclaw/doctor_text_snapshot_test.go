package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func doctorTextSnapshotRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func doctorTextSnapshotConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(doctorTextSnapshotRepoRoot(t), "internal", "config", "testdata", "doctor", "valid_minimal.yaml")
}

func normalizeDoctorSnapshotText(s, configAbs string) string {
	s = strings.ReplaceAll(s, configAbs, "<<CONFIG_PATH>>")
	// Normalize alternate absolute forms (symlinks, trailing slashes).
	if alt, err := filepath.EvalSymlinks(configAbs); err == nil && alt != configAbs {
		s = strings.ReplaceAll(s, alt, "<<CONFIG_PATH>>")
	}
	return s
}

func TestDoctorTextSnapshot_ValidMinimalSkipNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("builds CLI")
	}
	cfg := doctorTextSnapshotConfigPath(t)
	cfgAbs, err := filepath.Abs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfgAbs); err != nil {
		t.Fatal(err)
	}

	repoRoot := doctorTextSnapshotRepoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "assistclaw")
	build := exec.Command("go", "build", "-mod=vendor", "-tags", "fts5,assistclaw_localgemma", "-o", bin, "./cmd/assistclaw")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "doctor", "--config", cfgAbs, "--skip-network", "--log-level", "error")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("doctor: %v stderr=%q", err, stderr.String())
	}

	got := normalizeDoctorSnapshotText(stdout.String(), cfgAbs)
	snapPath := filepath.Join("testdata", "doctor_text_valid_minimal.snapshot.txt")
	wantBytes, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	want := string(wantBytes)

	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.WriteFile(snapPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", snapPath)
		return
	}

	if got != want {
		t.Fatalf("doctor text snapshot mismatch (run with UPDATE_SNAPSHOTS=1 to refresh):\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
