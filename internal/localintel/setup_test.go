package localintel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGGUFPath(t *testing.T) {
	got := DefaultGGUFPath("/tmp/ac")
	want := filepath.Join("/tmp/ac", "models", "gemma-4-e2b-it.gguf")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBootstrapGGUF_DownloadAndReuse(t *testing.T) {
	payload := []byte("gguf-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	sd := t.TempDir()
	res, err := BootstrapGGUF(context.Background(), BootstrapOptions{StateDir: sd, URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Downloaded {
		t.Fatalf("expected Downloaded=true, got %+v", res)
	}
	b, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(payload) {
		t.Fatalf("payload mismatch: %q", string(b))
	}

	res2, err := BootstrapGGUF(context.Background(), BootstrapOptions{StateDir: sd, URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Downloaded {
		t.Fatalf("expected reuse without download, got %+v", res2)
	}
}

func TestBootstrapGGUF_SHA256Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()
	_, err := BootstrapGGUF(context.Background(), BootstrapOptions{StateDir: t.TempDir(), URL: srv.URL, SHA256: "deadbeef"})
	if err == nil {
		t.Fatal("expected sha mismatch")
	}
}

func TestBootstrapGGUF_SHA256OK(t *testing.T) {
	payload := []byte("ok")
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	_, err := BootstrapGGUF(context.Background(), BootstrapOptions{StateDir: t.TempDir(), URL: srv.URL, SHA256: want})
	if err != nil {
		t.Fatal(err)
	}
}
