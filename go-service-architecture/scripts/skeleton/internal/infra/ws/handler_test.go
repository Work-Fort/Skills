package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHandleWSAcceptsConnection(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(HandleWS(hub, ctx))
	defer srv.Close()

	// Connect a WebSocket client.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	// Broadcast a message and verify the client receives it.
	hub.Broadcast([]byte(`{"id":"ntf_ws1","state":"sending"}`))

	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != `{"id":"ntf_ws1","state":"sending"}` {
		t.Errorf("msg = %q, want sending message", string(msg))
	}
}

func TestHandleWSClientDisconnect(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(HandleWS(hub, ctx))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Close the connection from the client side.
	conn.Close(websocket.StatusNormalClosure, "bye")

	// Allow time for the read pump to detect the close and
	// unregister the client.
	time.Sleep(50 * time.Millisecond)

	// Broadcast should not panic with no connected clients.
	hub.Broadcast([]byte(`{"id":"ntf_ws2","state":"delivered"}`))
}

func TestHandleWSMultipleClients(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(HandleWS(hub, ctx))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Connect two clients.
	conn1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial client 1: %v", err)
	}
	defer conn1.CloseNow()

	conn2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial client 2: %v", err)
	}
	defer conn2.CloseNow()

	// Allow registration to complete.
	time.Sleep(20 * time.Millisecond)

	hub.Broadcast([]byte(`{"id":"ntf_ws3","state":"delivered"}`))

	// Both clients should receive the message.
	for i, conn := range []*websocket.Conn{conn1, conn2} {
		readCtx, readCancel := context.WithTimeout(ctx, 200*time.Millisecond)
		_, msg, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			t.Fatalf("client %d read: %v", i+1, err)
		}
		if string(msg) != `{"id":"ntf_ws3","state":"delivered"}` {
			t.Errorf("client %d msg = %q, want delivered message", i+1, string(msg))
		}
	}
}

func TestHandleWSNonUpgradeRequest(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Send a plain HTTP GET (not a WebSocket upgrade).
	handler := HandleWS(hub, ctx)
	req := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// websocket.Accept writes an error for non-upgrade requests.
	if rec.Code == http.StatusSwitchingProtocols {
		t.Error("expected non-upgrade response, got 101")
	}
}
