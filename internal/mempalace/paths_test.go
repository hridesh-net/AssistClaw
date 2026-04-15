package mempalace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManagedPaths(t *testing.T) {
	const sd = "/tmp/ac"
	if g, w := filepath.Join(sd, "mempalace"), ManagedBaseDir(sd); g != w {
		t.Fatalf("ManagedBaseDir: got %q want %q", w, g)
	}
	if g, w := filepath.Join(sd, "mempalace", "venv"), ManagedVenvRoot(sd); g != w {
		t.Fatalf("ManagedVenvRoot: got %q want %q", w, g)
	}
}

func TestVenvInterpreter(t *testing.T) {
	t.Parallel()
	vr := t.TempDir()
	if runtime.GOOS == "windows" {
		got := VenvInterpreter(vr)
		want := filepath.Join(vr, "Scripts", "python.exe")
		if got != want {
			t.Fatalf("windows: got %q want %q", got, want)
		}
		return
	}
	bin := filepath.Join(vr, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	py3 := filepath.Join(bin, "python3")
	if err := os.WriteFile(py3, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	got := VenvInterpreter(vr)
	want := py3
	if got != want {
		t.Fatalf("unix: got %q want %q", got, want)
	}
}
