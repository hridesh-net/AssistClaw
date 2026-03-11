package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"tailscale.com/tsnet"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/webui"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Gateway handles local/remote auth via Bearer token
	},
}

// Server represents the Gateway HTTP and WebSocket server.
type Server struct {
	Hub        *Hub
	HTTPServer *http.Server
	Port       int
	Bind       string
	Token      string // Bearer token for auth (empty = no auth)
	Runner     *agent.Runner
	Version    string
	Tailscale  struct {
		Mode string
	}
	TS *tsnet.Server
}

// NewServer initializes a new Gateway server on the specified port.
func NewServer(port int) *Server {
	return &Server{
		Hub:  NewHub(),
		Port: port,
	}
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	go s.Hub.Run()

	mux := http.NewServeMux()

	// ── Auth middleware wrapper ───────────────────────────────────────────────
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		if s.Token == "" {
			return h
		}
		return func(w http.ResponseWriter, r *http.Request) {
			tok := r.Header.Get("Authorization")
			if !strings.HasPrefix(tok, "Bearer ") || strings.TrimPrefix(tok, "Bearer ") != s.Token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}

	// ── Static web UI ─────────────────────────────────────────────────────────
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(webui.Assets()))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Serve index.html from embedded FS
		data, err := webui.Assets().Open("index.html")
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		defer data.Close()
		http.ServeContent(w, r, "index.html", time.Time{}, data.(interface {
			http.File
		}))
	})

	// ── Health ────────────────────────────────────────────────────────────────
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ── API: Status ───────────────────────────────────────────────────────────
	mux.HandleFunc("/api/status", auth(s.handleStatus))

	// ── API: Chat (SSE streaming) ─────────────────────────────────────────────
	mux.HandleFunc("/api/chat", auth(s.handleChat))

	// ── WebSocket (legacy / channel use) ─────────────────────────────────────
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	addr := fmt.Sprintf(":%d", s.Port)
	if s.Bind == "tailnet" {
		s.TS = &tsnet.Server{
			Hostname: "assistclaw",
		}

		var ln net.Listener
		var err error

		if s.Tailscale.Mode == "funnel" {
			ln, err = s.TS.ListenFunnel("tcp", addr)
		} else {
			ln, err = s.TS.Listen("tcp", addr)
		}

		if err != nil {
			return fmt.Errorf("tailscale listen error: %w", err)
		}

		s.HTTPServer = &http.Server{Handler: mux}
		log.Printf("AssistClaw gateway + web UI listening via Tailscale (%s) on %s", s.Tailscale.Mode, addr)
		return s.HTTPServer.Serve(ln)
	}

	// Default loopback or LAN bind
	bindAddr := "127.0.0.1"
	if s.Bind == "lan" {
		bindAddr = "0.0.0.0"
	}
	fullAddr := fmt.Sprintf("%s%s", bindAddr, addr)

	s.HTTPServer = &http.Server{
		Addr:    fullAddr,
		Handler: mux,
	}

	log.Printf("AssistClaw gateway + web UI listening on http://%s", fullAddr)
	return s.HTTPServer.ListenAndServe()
}

// Stop safely shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("Stopping gateway...")
	if s.TS != nil {
		s.TS.Close()
	}
	if s.HTTPServer != nil {
		return s.HTTPServer.Shutdown(ctx)
	}
	return nil
}

// ── API Handlers ──────────────────────────────────────────────────────────────

// chatRequest is the JSON body for POST /api/chat.
type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

// sseEvent formats an SSE data line.
func sseEvent(eventType, content string) []byte {
	payload, _ := json.Marshal(map[string]string{"type": eventType, "content": content})
	return []byte("data: " + string(payload) + "\n\n")
}

func sseDone() []byte { return []byte("data: [DONE]\n\n") }

func sseToolEvent(eventType, name string) []byte {
	payload, _ := json.Marshal(map[string]string{"type": eventType, "name": name})
	return []byte("data: " + string(payload) + "\n\n")
}

// handleChat handles POST /api/chat, returning an SSE stream of tokens.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Runner == nil {
		http.Error(w, `{"error":"agent not initialised"}`, http.StatusServiceUnavailable)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Pick or create session-specific runner
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	sessionRunner := s.Runner.WithSession(sessionID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	done := make(chan struct{})

	handler := &sseStreamHandler{
		w:       w,
		flusher: flusher,
		done:    done,
	}

	go func() {
		sessionRunner.RunStream(ctx, memory.Message{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Role:      memory.RoleUser,
			Content:   req.Message,
			CreatedAt: time.Now(),
		}, handler)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// handleStatus handles GET /api/status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	model := ""
	if s.Runner != nil {
		// Runner doesn't expose model publicly, we embed it in Server
	}
	resp := map[string]interface{}{
		"status":  "ok",
		"version": s.Version,
		"pid":     os.Getpid(),
		"model":   model,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// ── SSE Stream Handler ────────────────────────────────────────────────────────

type sseStreamHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{}
}

func (h *sseStreamHandler) write(b []byte) {
	_, _ = h.w.Write(b)
	h.flusher.Flush()
}

func (h *sseStreamHandler) OnToken(token string) {
	h.write(sseEvent("token", token))
}

func (h *sseStreamHandler) OnToolCall(name string, _ json.RawMessage) {
	h.write(sseToolEvent("tool_start", name))
}

func (h *sseStreamHandler) OnToolResult(name string, _ string) {
	h.write(sseToolEvent("tool_end", name))
}

func (h *sseStreamHandler) OnDone(_ *agent.RunResult) {
	h.write(sseDone())
	close(h.done)
}

func (h *sseStreamHandler) OnError(err error) {
	h.write(sseEvent("error", err.Error()))
	h.write(sseDone())
	select {
	case <-h.done:
	default:
		close(h.done)
	}
}

// ── WebSocket (unchanged from original) ──────────────────────────────────────

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("gateway/server upgrade error:", err)
		return
	}

	clientID := uuid.New().String()
	client := &Client{
		ID:   clientID,
		Hub:  hub,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	client.Hub.register <- client

	go client.writePump()
	go client.readPump()
}
