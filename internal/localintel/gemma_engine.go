//go:build assistclaw_localgemma

package localintel

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dianlight/gollama.cpp"
	"github.com/jupiterrider/ffi"
)

var backendOnce sync.Once

func backendStart() error {
	var err error
	backendOnce.Do(func() {
		err = gollama.Backend_init()
	})
	return err
}

type gemmaEngine struct {
	path  string
	model gollama.LlamaModel
	lctx  gollama.LlamaContext
	mu    sync.Mutex
}

func newGemmaEngine(path string) (*gemmaEngine, error) {
	if !ffi.Available() {
		return nil, fmt.Errorf("localintel: ffi unavailable: %w", ffi.InitError())
	}
	if err := backendStart(); err != nil {
		return nil, err
	}
	mp := gollama.Model_default_params()
	mp.NGpuLayers = 0

	model, err := gollama.Model_load_from_file(path, mp)
	if err != nil {
		return nil, err
	}
	if model == 0 {
		return nil, fmt.Errorf("localintel: Model_load_from_file returned null for %s", path)
	}

	cp := gollama.Context_default_params()
	if cp.NCtx == 0 {
		cp.NCtx = 8192
	}
	if cp.NBatch == 0 {
		cp.NBatch = 512
	}
	lctx, err := gollama.Init_from_model(model, cp)
	if err != nil {
		gollama.Model_free(model)
		return nil, err
	}
	if lctx == 0 {
		gollama.Model_free(model)
		return nil, fmt.Errorf("localintel: Init_from_model returned null")
	}
	return &gemmaEngine{path: path, model: model, lctx: lctx}, nil
}

func (e *gemmaEngine) Available() bool { return true }

func (e *gemmaEngine) Close() error {
	if e.lctx != 0 {
		gollama.Free(e.lctx)
		e.lctx = 0
	}
	if e.model != 0 {
		gollama.Model_free(e.model)
		e.model = 0
	}
	return nil
}

func (e *gemmaEngine) Complete(ctx context.Context, req Request) (string, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 256
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if !gollama.Memory_clear(e.lctx, true) {
		return "", fmt.Errorf("localintel: failed to reset KV cache")
	}

	prompt := buildGemmaTurnPrompt(req.System, req.User)
	tokens, err := gollama.Tokenize(e.model, prompt, true, true)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 {
		return "", fmt.Errorf("localintel: empty prompt tokens")
	}

	pbatch := gollama.Batch_get_one(tokens)
	if err := gollama.Decode(e.lctx, pbatch); err != nil {
		return "", err
	}

	sampler := gollama.Sampler_init_greedy()
	if sampler == 0 {
		return "", fmt.Errorf("localintel: greedy sampler init failed")
	}
	defer gollama.Sampler_free(sampler)

	var out strings.Builder
	for i := 0; i < req.MaxTokens; i++ {
		select {
		case <-ctx.Done():
			return out.String(), ctx.Err()
		default:
		}

		tok := gollama.Sampler_sample(sampler, e.lctx, -1)
		if tok == gollama.LLAMA_TOKEN_NULL {
			break
		}

		piece := gollama.Token_to_piece(e.model, tok, true)
		if strings.Contains(piece, "<end_of_turn>") {
			break
		}
		out.WriteString(piece)

		next := []gollama.LlamaToken{tok}
		b := gollama.Batch_get_one(next)
		if err := gollama.Decode(e.lctx, b); err != nil {
			return out.String(), err
		}
	}
	return strings.TrimSpace(out.String()), nil
}

// buildGemmaTurnPrompt follows the common Gemma turn structure used across Gemma IT checkpoints.
// If the template diverges for a specific GGUF revision, adjust here once and keep the engine generic.
func buildGemmaTurnPrompt(system, user string) string {
	var b strings.Builder
	b.WriteString("<start_of_turn>user\n")
	if strings.TrimSpace(system) != "" {
		b.WriteString(system)
		b.WriteString("\n\n")
	}
	b.WriteString(user)
	b.WriteString("<end_of_turn>\n<start_of_turn>model\n")
	return b.String()
}
