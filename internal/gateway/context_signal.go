package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// contextSignalRequest is one external awareness signal, e.g. from a phone:
//
//	{"key": "user.location", "value": "home", "ttl_seconds": 600}
type contextSignalRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// handleContextSignal ingests external context signals into the awareness store.
func (s *Server) handleContextSignal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.Awareness == nil {
		http.Error(w, `{"error":"awareness store not configured"}`, http.StatusServiceUnavailable)
		return
	}
	var req contextSignalRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" || len(req.Key) > 128 || len(req.Value) > 1024 {
		http.Error(w, `{"error":"key required; key<=128 chars, value<=1024 chars"}`, http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute // external signals default to expiring rather than going stale forever
	}
	if req.Value == "" {
		s.Awareness.Delete(req.Key)
	} else {
		s.Awareness.Set(req.Key, req.Value, ttl)
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "key": req.Key})
}
