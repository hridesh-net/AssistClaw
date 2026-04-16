package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadForDoctor_ValidMinimal(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "doctor", "valid_minimal.yaml")
	cfg, issues, err := LoadForDoctor(path)
	if err != nil {
		t.Fatalf("LoadForDoctor: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config")
	}
	for _, i := range issues {
		if i.Severity == "error" {
			t.Errorf("unexpected error issue: %+v", i)
		}
	}
}

func TestLoadForDoctor_InvalidGatewayPort(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "doctor", "invalid_bad_gateway_port.yaml")
	cfg, issues, err := LoadForDoctor(path)
	if err != nil {
		t.Fatalf("LoadForDoctor: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config despite validation issues")
	}
	var saw bool
	for _, i := range issues {
		if i.Severity == "error" && strings.Contains(i.Message, "gateway.port") && strings.Contains(i.Message, "65535") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected gateway.port validation error, got %+v", issues)
	}
}

func TestLoadForDoctor_DeprecatedRoutingPrimary(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "doctor", "warn_deprecated_primary.yaml")
	_, issues, err := LoadForDoctor(path)
	if err != nil {
		t.Fatalf("LoadForDoctor: %v", err)
	}
	var saw bool
	for _, i := range issues {
		if i.Severity == "warn" && strings.Contains(i.Message, "routing.primary") && strings.Contains(i.Message, "routing.default") {
			saw = true
			if i.Line == 0 {
				t.Error("expected non-zero line for deprecated key")
			}
		}
	}
	if !saw {
		t.Fatalf("expected deprecated routing.primary warning, got %+v", issues)
	}
}

func TestLoadForDoctor_MissingVersionWarn(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "doctor", "warn_missing_version.yaml")
	_, issues, err := LoadForDoctor(path)
	if err != nil {
		t.Fatalf("LoadForDoctor: %v", err)
	}
	var saw bool
	for _, i := range issues {
		if i.Severity == "warn" && strings.Contains(i.Message, "version") && strings.Contains(i.Message, "version: 1") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected version unset warning, got %+v", issues)
	}
}

func TestLoadForDoctor_ParseError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("gateway: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadForDoctor(bad)
	if err == nil {
		t.Fatal("expected parse error")
	}
}
