package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverBundledGGUF_PrefersCanonicalInCWDModels(t *testing.T) {
	t.Parallel()
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	models := filepath.Join(tmp, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(models, "gemma-4-e2b-it.gguf")
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(models, "z-other.gguf"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := discoverBundledGGUF(filepath.Join(tmp, "state", "models", "gemma-4-e2b-it.gguf"))
	if !ok {
		t.Fatal("expected bundled gguf discovery")
	}
	gotSt, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	wantSt, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotSt, wantSt) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCopyFileAtomic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.gguf")
	dst := filepath.Join(tmp, "nested", "b.gguf")
	if err := os.WriteFile(src, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFileAtomic(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "abc" {
		t.Fatalf("content mismatch: %q", string(b))
	}
}
