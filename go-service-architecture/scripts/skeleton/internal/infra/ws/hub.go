package ws

import (
	"context"

	"github.com/coder/websocket"
)

// Client represents a connected WebSocket client. The hub manages the
// lifecycle; the HTTP upgrade handler creates clients and registers
// them with the hub.
type Client struct {
	hub  *Hub
	Conn *websocket.Conn
	send chan []byte
}

// NewClient creates a client for use by the HTTP upgrade handler. The
// send channel is buffered at 256 (REQ-009).
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		Conn: conn,
		send: make(chan []byte, 256),
	}
}

// WritePump sends messages from the client's send channel to the
// WebSocket connection. Exits when the context is cancelled or the
// send channel is closed (REQ-010, REQ-015).
func (c *Client) WritePump(ctx context.Context) {
	defer func() { _ = c.Conn.Close(websocket.StatusNormalClosure, "closing") }()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.Conn.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		}
	}
}

// ReadPump reads from the connection to detect disconnects. Payload
// is discarded since no client-to-server messages are expected.
// When a read error occurs, the client is unregistered (REQ-011).
// The read limit is set to 512 bytes to prevent memory exhaustion
// from malicious clients sending large frames (REQ-016).
func (c *Client) ReadPump(ctx context.Context) {
	defer func() { c.hub.unregister <- c }()
	c.Conn.SetReadLimit(512)
	for {
		if _, _, err := c.Conn.Read(ctx); err != nil {
			return
		}
	}
}

// Hub manages all connected WebSocket clients and broadcasts
// messages to them (REQ-004).
type Hub struct {
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

// NewHub creates a hub with a buffered broadcast channel (REQ-008).
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(msg []byte) {
	h.broadcast <- msg
}

// Run processes register, unregister, and broadcast events until the
// context is cancelled (REQ-006, REQ-007).
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client -- drop it (REQ-012).
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}
