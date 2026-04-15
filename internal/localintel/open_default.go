//go:build !assistclaw_localgemma

package localintel

// CompiledWithLocalGemma reports whether the binary was built with assistclaw_localgemma (in-process llama.cpp).
func CompiledWithLocalGemma() bool { return false }

func openEngine(_ Options) (Engine, error) {
	return noopEngine{}, nil
}
