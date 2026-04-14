package correlation

import (
	"context"
	"net/http"
	"testing"
)

func TestEnsureRequestID(t *testing.T) {
	t.Helper()
	ctx, rid := EnsureRequestID(context.Background())
	if rid == "" {
		t.Fatal("expected generated request_id")
	}
	if got := RequestID(ctx); got != rid {
		t.Fatalf("RequestID() = %q, want %q", got, rid)
	}

	ctx2, rid2 := EnsureRequestID(WithRequestID(context.Background(), "fixed"))
	if rid2 != "fixed" || RequestID(ctx2) != "fixed" {
		t.Fatalf("expected existing request_id to be preserved, got %q", rid2)
	}
}

func TestEnrichFromHTTPHeaders(t *testing.T) {
	t.Helper()
	h := make(http.Header)
	h.Set(HeaderRequestID, "req-1")
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	ctx := EnrichFromHTTPHeaders(context.Background(), h)
	if RequestID(ctx) != "req-1" {
		t.Fatalf("request_id = %q", RequestID(ctx))
	}
	if TraceID(ctx) != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id = %q", TraceID(ctx))
	}
}

func BenchmarkFields(b *testing.B) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "r1")
	ctx = WithSessionID(ctx, "s1")
	ctx = WithChannel(ctx, "telegram")
	ctx = WithTraceID(ctx, "t1")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Fields(ctx)
	}
}
