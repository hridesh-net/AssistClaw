package gateway

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"tailscale.com/tsnet"
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
	Bind       string
	Tailscale  struct {
		Mode string
	}
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

	addr := fmt.Sprintf(":%d", s.Port)
	if s.Bind == "tailnet" {
		ts := &tsnet.Server{
			Hostname: "assistclaw",
		}
		defer ts.Close()

		var ln net.Listener
		var err error

		if s.Tailscale.Mode == "funnel" {
			ln, err = ts.ListenFunnel("tcp", addr)
		} else {
			ln, err = ts.Listen("tcp", addr)
		}

		if err != nil {
			return fmt.Errorf("tailscale listen error: %w", err)
		}

		s.HTTPServer = &http.Server{
			Handler: mux,
		}
		log.Printf("Gateway control plane listening via Tailscale (%s) on %s", s.Tailscale.Mode, addr)
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

	log.Printf("Gateway control plane listening on ws://%s/ws", fullAddr)
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
