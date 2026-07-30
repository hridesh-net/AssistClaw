package gateway

import (
	"sync"

	"go.uber.org/zap"
)

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// shutdown signals Run() to exit.
	shutdown chan struct{}
	// wg tracks active client goroutines for clean drain.
	wg sync.WaitGroup

	log *zap.Logger
}

func NewHub(log *zap.Logger) *Hub {
	if log == nil {
		log = zap.NewNop()
	}
	return &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		clients:    make(map[*Client]bool),
		shutdown:   make(chan struct{}),
		log:        log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.wg.Add(1)
			h.log.Info("gateway/hub: client registered", zap.String("client_id", client.ID), zap.Int("total_active", len(h.clients)))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.SafeCloseSend()
				h.wg.Done()
				h.log.Info("gateway/hub: client unregistered", zap.String("client_id", client.ID), zap.Int("total_active", len(h.clients)))
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					client.SafeCloseSend()
					delete(h.clients, client)
					h.wg.Done()
				}
			}
		case <-h.shutdown:
			for client := range h.clients {
				client.SafeCloseSend()
				h.wg.Done()
			}
			h.clients = make(map[*Client]bool)
			return
		}
	}
}

// Stop signals the hub to shut down and waits for client goroutines to drain.
func (h *Hub) Stop() {
	close(h.shutdown)
	h.wg.Wait()
}
