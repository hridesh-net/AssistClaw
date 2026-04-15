package localintel

import "testing"

func TestNoopEngine_Available(t *testing.T) {
	var e Engine = noopEngine{}
	if e.Available() {
		t.Fatal("expected noop engine to report unavailable")
	}
	_, err := e.Complete(t.Context(), Request{User: "x"})
	if err != ErrNotCompiled {
		t.Fatalf("expected ErrNotCompiled, got %v", err)
	}
}
