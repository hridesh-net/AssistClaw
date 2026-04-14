package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"tailscale.com/tsnet"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/automation"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/observability/correlation"
	"github.com/assistclaw/assistclaw/internal/voice"
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
	Config     *config.Config
	Gmail      *automation.GmailWatcher
	Voice      *voice.Daemon
	Logger     *zap.Logger
}

// NewServer initializes a new Gateway server on the specified port.
func NewServer(port int) *Server {
	return &Server{
		Hub:  NewHub(),
		Port: port,
	}
}

func (s *Server) logger() *zap.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return zap.NewNop()
}

func (s *Server) withCorrelation(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := correlation.EnrichFromHTTPHeaders(r.Context(), r.Header)
		ctx, requestID := correlation.EnsureRequestID(ctx)
		r = r.WithContext(ctx)

		logFields := append(correlation.Fields(ctx),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
		)
		s.logger().Info("gateway inbound request", logFields...)
		w.Header().Set(correlation.HeaderRequestID, requestID)
		h(w, r)
	}
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	go s.Hub.Run()

	// Start automation workers if configured
	if s.Config != nil && s.Config.Gmail.Enabled {
		// NewGmailWatcher expects (config, logger) - we might need to pass the logger to Server
		// For now, let's assume we can use a basic logger or pass it in later.
		// Actually, let's just initialize it in main.go and set it on the Server.
		if s.Gmail != nil {
			if err := s.Gmail.Start(context.Background()); err != nil {
				log.Printf("gmail: failed to start watcher: %v", err)
			}
		}
	}

	mux := http.NewServeMux()

	// ── Agent-built static dashboards (served under /workspace/) ───────────
	// Serves ~/.assistclaw/workspace/public at /workspace/ — no Bearer token so
	// browsers can open links directly. Do not put secrets in this directory.
	if s.Config != nil {
		publicDir := filepath.Join(s.Config.StateDir, "workspace", "public")
		if err := os.MkdirAll(publicDir, 0o755); err != nil {
			log.Printf("gateway: workspace/public: %v", err)
		} else {
			mux.Handle("/workspace/", http.StripPrefix("/workspace/", http.FileServer(http.Dir(publicDir))))
		}
	}

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
	mux.HandleFunc("/api/status", auth(s.withCorrelation(s.handleStatus)))

	// ── API: Chat (SSE streaming) ─────────────────────────────────────────────
	mux.HandleFunc("/api/chat", auth(s.withCorrelation(s.handleChat)))

	// ── WebSocket (legacy / channel use) ─────────────────────────────────────
	mux.HandleFunc("/ws", s.withCorrelation(func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, s.logger(), w, r)
	}))
	
	// ── A2A Protocol ──────────────────────────────────────────────────────────
	mux.HandleFunc("/.well-known/agent.json", s.handleAgentCard)
	mux.HandleFunc("/api/a2a", s.handleA2A)

	// ── Webhooks ──────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/webhook/", auth(s.withCorrelation(s.handleWebhook)))

	if s.Gmail != nil {
		if err := s.Gmail.Start(context.Background()); err != nil {
			log.Printf("Error starting Gmail watcher: %v", err)
		}
	}

	if s.Voice != nil {
		if err := s.Voice.Start(context.Background()); err != nil {
			log.Printf("Error starting Voice daemon: %v", err)
		}
	}

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
		err := s.HTTPServer.Shutdown(ctx)
		if s.Gmail != nil {
			s.Gmail.Stop()
		}
		if s.Voice != nil {
			s.Voice.Stop()
		}
		return err
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
	ctx := correlation.WithSessionID(r.Context(), sessionID)
	ctx = correlation.WithChannel(ctx, "gateway")
	s.logger().Info("gateway chat request",
		append(correlation.Fields(ctx),
			zap.String("event", "chat.receive"),
		)...,
	)

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

// handleWebhook handles incoming webhooks, mapping them to agent prompts.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if s.Config == nil || !s.Config.Webhooks.Enabled {
		http.Error(w, `{"error":"webhooks disabled"}`, http.StatusForbidden)
		return
	}

	// Check for optional webhook token if configured
	if s.Config.Webhooks.Token != "" {
		tok := r.Header.Get("X-AssistClaw-Token")
		if tok != s.Config.Webhooks.Token {
			http.Error(w, `{"error":"invalid webhook token"}`, http.StatusUnauthorized)
			return
		}
	}

	// Get the preset path from the URL
	path := strings.TrimPrefix(r.URL.Path, "/api/webhook/")
	if path == "" {
		http.Error(w, `{"error":"invalid webhook path"}`, http.StatusBadRequest)
		return
	}

	// Find the mapping for this path
	var mapping *config.WebhookMapping
	for _, m := range s.Config.Webhooks.Mappings {
		if m.Path == path {
			mapping = &m
			break
		}
	}

	if mapping == nil {
		http.Error(w, `{"error":"webhook mapping not found"}`, http.StatusNotFound)
		return
	}

	// Read and parse the payload
	var payload map[string]interface{}
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error":"failed to decode json"}`, http.StatusBadRequest)
			return
		}
	}

	// Execute via agent
	prompt := mapping.PromptTemplate
	// Simple template replacement
	for k, v := range payload {
		placeholder := fmt.Sprintf("{{.%s}}", k)
		prompt = strings.ReplaceAll(prompt, placeholder, fmt.Sprintf("%v", v))
	}

	ctx := correlation.WithChannel(r.Context(), "webhook")
	s.logger().Info("webhook receive",
		append(correlation.Fields(ctx), zap.String("path", path))...,
	)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if s.Runner == nil {
		http.Error(w, `{"error":"agent not initialised"}`, http.StatusServiceUnavailable)
		return
	}

	sessionID := "webhook:" + path + ":" + uuid.New().String()
	ctx = correlation.WithSessionID(ctx, sessionID)
	runner := s.Runner.WithSession(sessionID)

	res, err := runner.Run(ctx, memory.Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      memory.RoleUser,
		Content:   prompt,
		CreatedAt: time.Now(),
	})

	if err != nil {
		s.logger().Error("webhook agent failed",
			append(correlation.Fields(ctx), zap.Error(err))...,
		)
		http.Error(w, fmt.Sprintf(`{"error":"agent execution failed","details":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"status":     "success",
		"iterations": res.Iterations,
	}

	// If delivery is requested, the agent's response is already in memory/last message
	// The implementation of 'deliver: true' usually involves the agent itself calling
	// a message tool, but if we want it automatic, we'd trigger it here.
	// For now, we return 200 OK.
	
	w.Header().Set("Content-Type", "application/json")
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

func serveWs(hub *Hub, logger *zap.Logger, w http.ResponseWriter, r *http.Request) {
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

	if logger != nil {
		logger.Info("gateway websocket connected",
			append(correlation.Fields(r.Context()), zap.String("client_id", clientID))...,
		)
	}

	client.Hub.register <- client

	go client.writePump()
	go client.readPump()
}
