package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all cross-origin requests for now (gateway handles local/remote auth)
	},
}

// Server represents the Gateway HTTP and WebSocket server.
type Server struct {
	Hub        *Hub
	HTTPServer *http.Server
	Port       int
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.Port) // Bind to loopback by default to match OpenClaw security
	s.HTTPServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Gateway control plane listening silently on ws://%s/ws", addr)
	return s.HTTPServer.ListenAndServe()
}

// Stop safely shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("Stopping gateway...")
	return s.HTTPServer.Shutdown(ctx)
}

// serveWs handles websocket requests from the peer.
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

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
