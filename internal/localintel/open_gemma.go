//go:build assistclaw_localgemma

package localintel

import (
	"fmt"
	"os"
	"strings"
)

// CompiledWithLocalGemma reports whether the binary was built with assistclaw_localgemma (in-process llama.cpp).
func CompiledWithLocalGemma() bool { return true }

func openEngine(opt Options) (Engine, error) {
	gguf := strings.TrimSpace(opt.GGUFPath)
	if gguf == "" {
		gguf = strings.TrimSpace(os.Getenv("ASSISTCLAW_LOCAL_GEMMA_GGUF"))
	}
	path, err := ResolveGGUFPath(opt.CacheDir, gguf)
	if err != nil {
		return nil, err
	}
	eng, err := newGemmaEngine(path)
	if err != nil {
		return nil, fmt.Errorf("localintel: open engine: %w", err)
	}
	return eng, nil
}
