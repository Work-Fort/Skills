package ws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHubRegisterAndBroadcast(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Create a fake client with a buffered send channel.
	send := make(chan []byte, 256)
	client := &Client{hub: hub, send: send}

	hub.Register(client)

	// Allow goroutine to process registration.
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message.
	hub.Broadcast([]byte(`{"id":"ntf_1","state":"delivered"}`))

	select {
	case msg := <-send:
		if string(msg) != `{"id":"ntf_1","state":"delivered"}` {
			t.Errorf("msg = %q, want delivered message", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestHubUnregister(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	send := make(chan []byte, 256)
	client := &Client{hub: hub, send: send}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond)

	// Broadcast should not panic or send to closed channel -- the
	// unregistered client's send channel is closed by the hub.
	hub.Broadcast([]byte(`{"id":"ntf_2","state":"sending"}`))

	// The send channel was closed by unregister; reading should
	// return the zero value immediately.
	select {
	case _, ok := <-send:
		if ok {
			t.Error("expected send channel to be closed after unregister")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out -- send channel should be closed")
	}
}

func TestHubDropsSlowClient(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Create a client with a tiny send channel to simulate a slow
	// reader.
	slowSend := make(chan []byte, 1)
	slowClient := &Client{hub: hub, send: slowSend}

	// Create a healthy client.
	fastSend := make(chan []byte, 256)
	fastClient := &Client{hub: hub, send: fastSend}

	hub.Register(slowClient)
	hub.Register(fastClient)
	time.Sleep(10 * time.Millisecond)

	// Fill the slow client's send channel.
	slowSend <- []byte("fill")

	// Broadcast should drop the slow client and still deliver to
	// the fast client.
	hub.Broadcast([]byte(`{"id":"ntf_3","state":"failed"}`))

	select {
	case msg := <-fastSend:
		if string(msg) != `{"id":"ntf_3","state":"failed"}` {
			t.Errorf("fast client msg = %q, want failed message", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("fast client timed out waiting for broadcast")
	}

	// The slow client's send channel should be closed.
	select {
	case _, ok := <-slowSend:
		// Drain the "fill" message first.
		if ok {
			select {
			case _, ok2 := <-slowSend:
				if ok2 {
					t.Error("slow client send channel should be closed")
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for slow client channel close")
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out reading slow client channel")
	}
}

func TestHubContextCancellation(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Run exited as expected.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("hub.Run did not exit after context cancellation")
	}
}

func TestReadPumpRejectsLargeFrame(t *testing.T) {
	hub := NewHub(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(HandleWS(hub, ctx, []string{"127.0.0.1:*"}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	// Send a message larger than 512 bytes.
	largeMsg := make([]byte, 1024)
	for i := range largeMsg {
		largeMsg[i] = 'A'
	}
	err = conn.Write(ctx, websocket.MessageText, largeMsg)
	if err != nil {
		// Write may fail if the connection is already closing.
		return
	}

	// The next read should fail because the server closed the
	// connection after the oversized frame.
	_, _, err = conn.Read(ctx)
	if err == nil {
		t.Fatal("expected read error after oversized frame, got nil")
	}
}

func TestHubRejectsRegistrationAtCapacity(t *testing.T) {
	hub := NewHub(2) // limit to 2 connections
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Register two clients (at capacity).
	send1 := make(chan []byte, 256)
	client1 := &Client{hub: hub, send: send1}
	hub.Register(client1)

	send2 := make(chan []byte, 256)
	client2 := &Client{hub: hub, send: send2}
	hub.Register(client2)

	time.Sleep(10 * time.Millisecond)

	// Third registration should be rejected.
	send3 := make(chan []byte, 256)
	client3 := &Client{hub: hub, send: send3}
	hub.Register(client3)

	time.Sleep(10 * time.Millisecond)

	// The rejected client's send channel should be closed.
	select {
	case _, ok := <-send3:
		if ok {
			t.Error("expected rejected client's send channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for rejected client channel close")
	}

	// The existing clients should still receive broadcasts.
	hub.Broadcast([]byte(`{"id":"ntf_cap","state":"delivered"}`))

	select {
	case msg := <-send1:
		if string(msg) != `{"id":"ntf_cap","state":"delivered"}` {
			t.Errorf("client 1 msg = %q, want delivered message", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("client 1 timed out waiting for broadcast")
	}

	select {
	case msg := <-send2:
		if string(msg) != `{"id":"ntf_cap","state":"delivered"}` {
			t.Errorf("client 2 msg = %q, want delivered message", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("client 2 timed out waiting for broadcast")
	}
}

func TestHubAcceptsBelowCapacity(t *testing.T) {
	hub := NewHub(2) // limit to 2 connections
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Register one client (below capacity).
	send1 := make(chan []byte, 256)
	client1 := &Client{hub: hub, send: send1}
	hub.Register(client1)

	time.Sleep(10 * time.Millisecond)

	// Second registration should succeed.
	send2 := make(chan []byte, 256)
	client2 := &Client{hub: hub, send: send2}
	hub.Register(client2)

	time.Sleep(10 * time.Millisecond)

	// Both clients should receive broadcasts.
	hub.Broadcast([]byte(`{"id":"ntf_below","state":"sending"}`))

	for i, send := range []chan []byte{send1, send2} {
		select {
		case msg := <-send:
			if string(msg) != `{"id":"ntf_below","state":"sending"}` {
				t.Errorf("client %d msg = %q, want sending message", i+1, string(msg))
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("client %d timed out waiting for broadcast", i+1)
		}
	}
}

func TestHubAcceptsAfterUnregister(t *testing.T) {
	hub := NewHub(1) // limit to 1 connection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Register one client (at capacity).
	send1 := make(chan []byte, 256)
	client1 := &Client{hub: hub, send: send1}
	hub.Register(client1)

	time.Sleep(10 * time.Millisecond)

	// Unregister to free a slot.
	hub.Unregister(client1)

	time.Sleep(10 * time.Millisecond)

	// New registration should succeed.
	send2 := make(chan []byte, 256)
	client2 := &Client{hub: hub, send: send2}
	hub.Register(client2)

	time.Sleep(10 * time.Millisecond)

	hub.Broadcast([]byte(`{"id":"ntf_free","state":"pending"}`))

	select {
	case msg := <-send2:
		if string(msg) != `{"id":"ntf_free","state":"pending"}` {
			t.Errorf("msg = %q, want pending message", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast after unregister + re-register")
	}
}
