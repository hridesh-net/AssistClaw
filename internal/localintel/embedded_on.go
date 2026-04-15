//go:build assistclaw_embedlocalgemma

package localintel

import _ "embed"

//go:embed embedded/gemma-4-e2b-it.gguf
var Gemma4E2BGGUF []byte
