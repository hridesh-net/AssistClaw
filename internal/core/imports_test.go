package core

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoreImportsOnlyStdlib enforces the central architectural invariant of the
// revamp: internal/core is the contract layer and must depend on nothing —
// not a third-party module, not another internal package. Every import path in
// every non-test file here must be a standard-library package.
//
// The stdlib heuristic is the canonical one the Go toolchain itself uses: a
// package path whose first segment contains a "." (e.g. "github.com/...") is
// external; a bare first segment (e.g. "context", "encoding/json") is stdlib.
func TestCoreImportsOnlyStdlib(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read core dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first := path
			if i := strings.IndexByte(path, '/'); i >= 0 {
				first = path[:i]
			}
			if strings.Contains(first, ".") {
				t.Errorf("%s imports non-stdlib package %q — internal/core must import only the standard library", name, path)
			}
		}
	}
}
