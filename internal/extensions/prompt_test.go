package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptAppendix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	if err := os.WriteFile(p, []byte("hello **world**"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := PromptAppendix(dir, []string{"note.md"})
	if !strings.Contains(out, "hello **world**") {
		t.Fatalf("expected content in appendix: %q", out)
	}
	if !strings.Contains(out, "Extension context") {
		t.Fatalf("expected header: %q", out)
	}
}

func TestPromptAppendixEmpty(t *testing.T) {
	t.Parallel()
	if PromptAppendix(t.TempDir(), nil) != "" {
		t.Fatal("expected empty")
	}
}
