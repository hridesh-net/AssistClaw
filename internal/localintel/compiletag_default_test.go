//go:build !assistclaw_localgemma

package localintel

import "testing"

func TestCompiledWithLocalGemma_defaultBinary(t *testing.T) {
	if CompiledWithLocalGemma() {
		t.Fatal("expected CompiledWithLocalGemma false without assistclaw_localgemma build tag")
	}
}
