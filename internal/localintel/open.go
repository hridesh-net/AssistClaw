package localintel

import (
	"os"
	"path/filepath"
)

// Open constructs a local Gemma engine when built with assistclaw_localgemma; otherwise returns a
// noop engine whose Complete returns ErrNotCompiled.
func Open(opt Options) (Engine, error) {
	if opt.CacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		opt.CacheDir = filepath.Join(base, "assistclaw")
	}
	return openEngine(opt)
}
