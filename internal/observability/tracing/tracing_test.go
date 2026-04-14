package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpanNamesExist_InMemoryExporter(t *testing.T) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(recorder)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	names := []string{
		"gateway.receive_message",
		"agent.enqueue_message",
		"agent.model_call",
		"agent.send_reply",
		"adapter.send",
	}

	for _, name := range names {
		ctx, span := StartSpan(context.Background(), name)
		_, child := StartSpan(ctx, name+".child")
		child.End()
		span.End()
	}

	spans := recorder.Ended()
	seen := map[string]bool{}
	for _, s := range spans {
		seen[s.Name()] = true
	}
	for _, name := range names {
		if !seen[name] {
			t.Fatalf("expected span %q to be recorded", name)
		}
	}
}
