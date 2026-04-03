package gateway

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

// MessagePayload represents a JSON message sent over the WebSocket.
// It loosely follows a simple WebSocket JSON envelope for agent streaming.
type MessagePayload struct {
	Type    string          `json:"type"`              // e.g., "ping", "message", "status", "stream_token", "tool_call"
	Session string          `json:"session,omitempty"` // Target or source session ID
	Data    json.RawMessage `json:"data,omitempty"`    // Payload specific to the type
}

// Client represents a connected WebSocket client (e.g., WebChat UI, macOS app, Channel webhook).
type Client struct {
	ID   string
	Conn *websocket.Conn
	Send chan []byte
	Hub  *Hub
}
