package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDoctorOutputJSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := doctorOutput{
		SchemaVersion: 1,
		ConfigPath:    "/tmp/assistclaw.yaml",
		ExitCode:      1,
		Checks: []doctorCheck{
			{ID: "config.validate", Severity: "ok", Message: "ok", Details: map[string]string{"config_path": "/tmp/assistclaw.yaml"}},
			{ID: "config.deprecated", Severity: "warn", Message: "legacy", Details: map[string]string{"file": "x.yaml", "line": "3", "column": "1"}},
		},
	}
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var out doctorOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.SchemaVersion != 1 || out.ExitCode != 1 || out.ConfigPath != in.ConfigPath {
		t.Fatalf("round trip: %+v", out)
	}
	if len(out.Checks) != 2 || out.Checks[0].Details["config_path"] != "/tmp/assistclaw.yaml" {
		t.Fatalf("checks: %+v", out.Checks)
	}
}

func TestDoctorJSONSubprocess_SkipNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI binary")
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	cfg := filepath.Join(repoRoot, "internal", "config", "testdata", "doctor", "valid_minimal.yaml")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "assistclaw")
	build := exec.Command("go", "build", "-o", bin, "./cmd/assistclaw")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "doctor", "--config", cfg, "--skip-network", "--no-input", "--json")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("doctor: %v\nstderr: %s", err, stderr.String())
	}

	var doc doctorOutput
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("parse JSON: %v\nstdout: %s", err, stdout.String())
	}
	if doc.SchemaVersion < 1 {
		t.Fatalf("schema_version: %d", doc.SchemaVersion)
	}
	if doc.ExitCode != 0 {
		t.Fatalf("exit_code: %d (stderr: %s)", doc.ExitCode, stderr.String())
	}
	if doc.ConfigPath != cfg {
		t.Fatalf("config_path: got %q want %q", doc.ConfigPath, cfg)
	}
	if len(doc.Checks) == 0 {
		t.Fatal("expected checks")
	}
	for _, c := range doc.Checks {
		if c.ID == "" || c.Severity == "" || c.Message == "" {
			t.Fatalf("invalid check: %+v", c)
		}
	}
}
