package localintel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeGGUF_EmptyWeights(t *testing.T) {
	if len(Gemma4E2BGGUF) != 0 {
		t.Skip("embedded weights present")
	}
	_, err := MaterializeGGUF(t.TempDir())
	if err != ErrEmbeddedWeightsEmpty {
		t.Fatalf("expected ErrEmbeddedWeightsEmpty, got %v", err)
	}
}

func TestResolveGGUFPath_file(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.gguf")
	if err := os.WriteFile(p, []byte("not-a-real-gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveGGUFPath(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("expected %q, got %q", p, got)
	}
}

func TestResolveGGUFPath_missingEmbed(t *testing.T) {
	if len(Gemma4E2BGGUF) != 0 {
		t.Skip("embedded weights present")
	}
	_, err := ResolveGGUFPath(t.TempDir(), "")
	if err != ErrNoWeights {
		t.Fatalf("expected ErrNoWeights, got %v", err)
	}
}
