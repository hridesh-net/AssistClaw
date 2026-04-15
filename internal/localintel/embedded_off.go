//go:build !assistclaw_embedlocalgemma

package localintel

// Gemma4E2BGGUF is populated when building with -tags assistclaw_embedlocalgemma and a GGUF at
// internal/localintel/embedded/gemma-4-e2b-it.gguf.
var Gemma4E2BGGUF []byte
