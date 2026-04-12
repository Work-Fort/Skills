package ws

import (
	"context"
	"testing"
	"time"
)

func TestHubRegisterAndBroadcast(t *testing.T) {
	hub := NewHub()
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
	hub := NewHub()
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
	hub := NewHub()
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
	hub := NewHub()
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
