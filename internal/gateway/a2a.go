package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/google/uuid"
)

// AgentCard represents the A2A Agent Card structure.
type AgentCard struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Endpoint     string   `json:"endpoint"`
	Capabilities []string `json:"capabilities"`
	AgentID      string   `json:"agent_id,omitempty"`
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// A2AMessageParams represents the parameters for message/send and message/stream.
type A2AMessageParams struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

// A2ATaskResult represents the result of a message/send request.
type A2ATaskResult struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	name := s.Config.A2A.Name
	if name == "" {
		name = "AssistClaw"
	}
	desc := s.Config.A2A.Description
	if desc == "" {
		desc = "Autonomous AI Agent"
	}

	card := AgentCard{
		Name:         name,
		Description:  desc,
		Version:      s.Version,
		AgentID:      s.Config.A2A.AgentID,
		Capabilities: []string{
			"messaging",
			"streaming",
			"task-management",
		},
		Endpoint: fmt.Sprintf("http://%s/api/a2a", r.Host),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func (s *Server) handleA2A(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONRPCError(w, nil, -32700, "Parse error")
		return
	}

	switch req.Method {
	case "message/send":
		s.handleA2AMessageSend(w, r, &req)
	case "message/stream":
		s.handleA2AMessageStream(w, r, &req)
	default:
		s.sendJSONRPCError(w, req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleA2AMessageSend(w http.ResponseWriter, r *http.Request, req *JSONRPCRequest) {
	var params A2AMessageParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendJSONRPCError(w, req.ID, -32602, "Invalid params")
		return
	}

	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	sessionRunner := s.Runner.WithSession(sessionID)

	ctx := r.Context()
	res, err := sessionRunner.Run(ctx, memory.Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      memory.RoleUser,
		Content:   params.Message,
		CreatedAt: time.Now(),
	})

	if err != nil {
		s.sendJSONRPCError(w, req.ID, -32000, err.Error())
		return
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: A2ATaskResult{
			Content:   res.Response,
			CreatedAt: time.Now(),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleA2AMessageStream(w http.ResponseWriter, r *http.Request, req *JSONRPCRequest) {
	var params A2AMessageParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendJSONRPCError(w, req.ID, -32602, "Invalid params")
		return
	}

	// SSE Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	sessionRunner := s.Runner.WithSession(sessionID)

	ctx := r.Context()
	done := make(chan struct{})
	handler := &a2aSSEHandler{
		w:       w,
		flusher: flusher,
		done:    done,
		id:      req.ID,
	}

	go func() {
		sessionRunner.RunStream(ctx, memory.Message{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Role:      memory.RoleUser,
			Content:   params.Message,
			CreatedAt: time.Now(),
		}, handler)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (s *Server) sendJSONRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type a2aSSEHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{}
	id      interface{}
}

func (h *a2aSSEHandler) OnToken(token string) {
	payload, _ := json.Marshal(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      h.id,
		Result: map[string]string{
			"type":    "token",
			"content": token,
		},
	})
	fmt.Fprintf(h.w, "data: %s\n\n", string(payload))
	h.flusher.Flush()
}

func (h *a2aSSEHandler) OnToolCall(name string, _ json.RawMessage)    {}
func (h *a2aSSEHandler) OnToolResult(name string, _ string)          {}
func (h *a2aSSEHandler) OnDone(_ *agent.RunResult) {
	payload, _ := json.Marshal(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      h.id,
		Result: map[string]string{
			"type": "done",
		},
	})
	fmt.Fprintf(h.w, "data: %s\n\n", string(payload))
	h.flusher.Flush()
	close(h.done)
}
func (h *a2aSSEHandler) OnError(err error) {
	payload, _ := json.Marshal(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      h.id,
		Error: &JSONRPCError{
			Code:    -32000,
			Message: err.Error(),
		},
	})
	fmt.Fprintf(h.w, "data: %s\n\n", string(payload))
	h.flusher.Flush()
	close(h.done)
}
