package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCorrelation_GeneratesRequestID(t *testing.T) {
	t.Helper()
	s := NewServer(0)
	handler := s.withCorrelation(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected generated X-Request-Id response header")
	}
}

func TestWithCorrelation_PreservesIncomingRequestID(t *testing.T) {
	t.Helper()
	s := NewServer(0)
	handler := s.withCorrelation(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", "req-fixed-123")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got != "req-fixed-123" {
		t.Fatalf("X-Request-Id = %q, want %q", got, "req-fixed-123")
	}
}
