package ws

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
)

// HandleWS returns an http.HandlerFunc that upgrades HTTP connections
// to WebSocket and registers clients with the hub (REQ-001, REQ-003,
// REQ-020).
//
// The connCtx parameter provides the lifecycle context for all
// connections. After websocket.Accept hijacks the connection,
// r.Context() is unreliable (it may be cancelled when the HTTP
// handler returns). Use a context derived from the hub's lifecycle
// instead.
//
// The allowedOrigins parameter is forwarded to
// websocket.AcceptOptions{OriginPatterns: allowedOrigins}. Passing
// nil disables origin checking and is prohibited (REQ-003).
func HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: allowedOrigins,
		})
		if err != nil {
			return // Accept writes the HTTP error response.
		}
		client := NewClient(hub, conn)
		hub.Register(client)

		go client.WritePump(connCtx)
		client.ReadPump(connCtx) // blocks until disconnect
	}
}
