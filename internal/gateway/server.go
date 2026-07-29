package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"net/http/pprof"
	"tailscale.com/tsnet"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/automation"
	"github.com/assistclaw/assistclaw/internal/awareness"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/observability/correlation"
	"github.com/assistclaw/assistclaw/internal/observability/metrics"
	obstracing "github.com/assistclaw/assistclaw/internal/observability/tracing"
	"github.com/assistclaw/assistclaw/internal/provider"
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
	TS     *tsnet.Server
	Config *config.Config
	Gmail  *automation.GmailWatcher
	Voice  *voice.Daemon
	Logger *zap.Logger

	// Health check dependencies (optional).
	MemMgr     *memory.Manager
	Provider   provider.Provider
	Proactive  interface{ IsRunning() bool }

	// Awareness receives external context signals (phone, sensors) via
	// POST /api/context/signal. Optional.
	Awareness *awareness.Store
}

// NewServer initializes a new Gateway server on the specified port.
func NewServer(port int) *Server {
	return &Server{
		Hub:  NewHub(nil),
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
func (s *Server) Start(ctx context.Context) error {
	if s.Hub != nil {
		s.Hub.log = s.logger()
		go s.Hub.Run()
	}
	metrics.Default().SetGatewayUp(true)

	// Start automation workers if configured
	if s.Config != nil && s.Config.Gmail.Enabled {
		if s.Gmail != nil {
			if err := s.Gmail.Start(ctx); err != nil {
				s.logger().Warn("gmail: failed to start watcher", zap.Error(err))
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
			s.logger().Warn("gateway: workspace/public", zap.Error(err))
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
	assetsFS, err := webui.Assets()
	if err != nil {
		s.logger().Error("failed to load embedded assets", zap.Error(err))
	} else {
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Serve index.html from embedded FS
		if assetsFS == nil {
			http.Error(w, "assets unavailable", http.StatusInternalServerError)
			return
		}
		data, err := assetsFS.Open("index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer data.Close()
		http.ServeContent(w, r, "index.html", time.Time{}, data.(interface {
			http.File
		}))
	})

	// ── Health ────────────────────────────────────────────────────────────────
	mux.HandleFunc("/livez", s.handleLivez)
	mux.HandleFunc("/readyz", s.handleReadyz)

	// ── API: Status ───────────────────────────────────────────────────────────
	mux.HandleFunc("/api/status", auth(s.withCorrelation(s.handleStatus)))
	mux.HandleFunc("/metrics", auth(s.withCorrelation(s.handleMetrics)))

	// ── API: Chat (SSE streaming) ─────────────────────────────────────────────
	mux.HandleFunc("/api/chat", auth(s.withCorrelation(s.handleChat)))

	// ── API: Context signals (phone/sensor → awareness store) ────────────────
	mux.HandleFunc("/api/context/signal", auth(s.withCorrelation(s.handleContextSignal)))

	// ── WebSocket (legacy / channel use) ─────────────────────────────────────
	mux.HandleFunc("/ws", s.withCorrelation(func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, s.logger(), w, r)
	}))

	// ── A2A Protocol ──────────────────────────────────────────────────────────
	mux.HandleFunc("/.well-known/agent.json", s.handleAgentCard)
	mux.HandleFunc("/api/a2a", s.handleA2A)

	// ── Webhooks ──────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/webhook/", auth(s.withCorrelation(s.handleWebhook)))

	// ── Debug pprof ───────────────────────────────────────────────────────────
	if s.Config != nil && s.Config.Gateway.Debug {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	if s.Gmail != nil {
		if err := s.Gmail.Start(ctx); err != nil {
			s.logger().Warn("error starting Gmail watcher", zap.Error(err))
		}
	}

	if s.Voice != nil {
		if err := s.Voice.Start(ctx); err != nil {
			s.logger().Warn("error starting Voice daemon", zap.Error(err))
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

		s.HTTPServer = &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      0, // WebSocket connections manage their own write deadlines
			IdleTimeout:       120 * time.Second,
		}
		s.logger().Info("gateway listening via Tailscale", zap.String("mode", s.Tailscale.Mode), zap.String("addr", addr))
		return s.HTTPServer.Serve(ln)
	}

	// Default loopback or LAN bind
	bindAddr := "127.0.0.1"
	if s.Bind == "lan" {
		bindAddr = "0.0.0.0"
	}
	fullAddr := fmt.Sprintf("%s%s", bindAddr, addr)

	s.HTTPServer = &http.Server{
		Addr:              fullAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // WebSocket connections manage their own write deadlines
		IdleTimeout:       120 * time.Second,
	}

	s.logger().Info("gateway listening", zap.String("addr", fullAddr))
	return s.HTTPServer.ListenAndServe()
}

// Stop safely shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.logger().Info("stopping gateway")
	metrics.Default().SetGatewayUp(false)
	if s.Hub != nil {
		s.Hub.Stop()
	}
	if s.TS != nil {
		s.TS.Close()
	}
	var err error
	if s.HTTPServer != nil {
		err = s.HTTPServer.Shutdown(ctx)
	}
	if s.Gmail != nil {
		s.Gmail.Stop()
	}
	if s.Voice != nil {
		s.Voice.Stop()
	}
	return err
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(metrics.Default().RenderPrometheus()))
}

func (s *Server) handleLivez(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	checks := make(map[string]string)
	ready := true

	if s.MemMgr != nil {
		if err := s.MemMgr.Episodic.Ping(); err != nil {
			checks["memory"] = "error: " + err.Error()
			ready = false
		} else {
			checks["memory"] = "ok"
		}
	} else {
		checks["memory"] = "not configured"
	}

	if s.Provider != nil {
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Provider.HealthCheck(pingCtx); err != nil {
			checks["provider"] = "error: " + err.Error()
			ready = false
		} else {
			checks["provider"] = "ok"
		}
	} else {
		checks["provider"] = "not configured"
	}

	if s.Proactive != nil {
		if s.Proactive.IsRunning() {
			checks["proactive"] = "ok"
		} else {
			checks["proactive"] = "stopped"
			ready = false
		}
	} else {
		checks["proactive"] = "not configured"
	}

	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": checks})
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
	spanCtx, span := obstracing.StartSpan(ctx, "gateway.receive_message")
	defer span.End()
	s.logger().Info("gateway chat request",
		append(correlation.Fields(spanCtx),
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

	handler := &sseStreamHandler{
		w:       w,
		flusher: flusher,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessionRunner.RunStream(spanCtx, memory.Message{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Role:      memory.RoleUser,
			Content:   req.Message,
			CreatedAt: time.Now(),
		}, handler)
	}()

	wg.Wait()
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
}

func (h *sseStreamHandler) OnError(err error) {
	h.write(sseEvent("error", err.Error()))
	h.write(sseDone())
}

// ── WebSocket (unchanged from original) ──────────────────────────────────────

func serveWs(hub *Hub, logger *zap.Logger, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if logger != nil {
			logger.Warn("gateway websocket upgrade error", zap.Error(err))
		}
		return
	}

	clientID := uuid.New().String()
	client := &Client{
		ID:     clientID,
		Hub:    hub,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		logger: logger,
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
