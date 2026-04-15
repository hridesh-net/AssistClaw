package agent

import (
	"context"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/localintel"
)

const defaultLocalIntelSystem = `You are a small on-device model (Gemma 4 E2B). Produce a terse advisory (max ~16 short lines) for a stronger cloud model that will execute the task.
Cover: suggested next actions, risks/unknowns, whether tools or skills likely apply, and any clarifying question worth asking.
Be concrete; do not refuse based on your size.`

func (r *Runner) prepareLocalIntelScratch(ctx context.Context, userMessage string) {
	r.localIntelScratch = ""
	if !r.cfg.LocalIntel.Enabled {
		return
	}
	eng := r.ensureLocalIntelEngine()
	if eng == nil {
		return
	}
	sys := strings.TrimSpace(r.cfg.LocalIntel.SystemPrompt)
	if sys == "" {
		sys = defaultLocalIntelSystem
	}
	maxTok := r.cfg.LocalIntel.MaxTokens
	if maxTok <= 0 {
		maxTok = 256
	}
	var ub strings.Builder
	ub.WriteString("Latest user message:\n")
	ub.WriteString(userMessage)
	if sk := strings.TrimSpace(r.cfg.ActiveSkillsContext); sk != "" {
		ub.WriteString("\n\n## Active skills map (truncated)\n")
		ub.WriteString(truncateUTF8(sk, 8000))
	}
	out, err := eng.Complete(ctx, localintel.Request{
		System:    sys,
		User:      ub.String(),
		MaxTokens: maxTok,
	})
	if err != nil {
		r.log.Warn("local_intel: completion failed", zap.Error(err))
		return
	}
	if strings.TrimSpace(out) == "" {
		return
	}
	r.localIntelScratch = out
}

func (r *Runner) ensureLocalIntelEngine() localintel.Engine {
	if !r.cfg.LocalIntel.Enabled {
		return nil
	}
	r.localIntelOnce.Do(func() {
		opts := localintel.Options{
			CacheDir: r.cfg.LocalIntel.CacheDir,
			GGUFPath: r.cfg.LocalIntel.GGUFPath,
		}
		eng, err := localintel.Open(opts)
		r.localIntelEng = eng
		r.localIntelOpenErr = err
		if err != nil {
			r.log.Warn("local_intel: open", zap.Error(err))
			return
		}
		if eng == nil || !eng.Available() {
			r.log.Info("local_intel: enabled but engine unavailable — rebuild with assistclaw_localgemma and/or set gguf_path / ASSISTCLAW_LOCAL_GEMMA_GGUF / embed weights")
		}
	})
	if r.localIntelOpenErr != nil {
		return nil
	}
	if r.localIntelEng == nil || !r.localIntelEng.Available() {
		return nil
	}
	return r.localIntelEng
}

func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
