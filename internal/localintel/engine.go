package localintel

import "context"

// Request is a single-turn local completion (expand later for multi-turn and tool JSON).
type Request struct {
	System    string
	User      string
	MaxTokens int
}

// Engine runs Gemma 4 E2B (or compatible GGUF) fully inside the process when built with assistclaw_localgemma.
type Engine interface {
	Complete(ctx context.Context, req Request) (string, error)
	Close() error
	// Available is true when this binary includes the in-process engine and a GGUF was resolved.
	Available() bool
}

// Options configures how the GGUF is resolved and how the runtime uses the model.
type Options struct {
	// CacheDir is used to materialize embedded weights (e.g. os.UserCacheDir() + "/assistclaw").
	CacheDir string
	// GGUFPath overrides embedded weights when non-empty.
	GGUFPath string
}

type noopEngine struct{}

func (noopEngine) Complete(context.Context, Request) (string, error) { return "", ErrNotCompiled }
func (noopEngine) Close() error                                      { return nil }
func (noopEngine) Available() bool                                   { return false }
