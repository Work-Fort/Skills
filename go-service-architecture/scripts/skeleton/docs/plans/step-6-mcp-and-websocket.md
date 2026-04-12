---
type: plan
step: "6"
title: "MCP and WebSocket"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "6"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-1-project-skeleton
  - step-2-cli-and-database
  - step-3-notification-delivery
  - step-4-state-machine
  - step-5-reset-list-postgres
---

# Step 6: MCP and WebSocket

## Overview

Expose the notification service through two additional interfaces: MCP
tools for AI agent integration and a WebSocket endpoint for real-time
dashboard updates. Both reuse the existing service/store layer --
no domain logic is duplicated.

After this step:

- AI agents can call `send_notification`, `reset_notification`,
  `list_notifications`, and `check_health` via the MCP protocol at
  `/mcp`.
- A stdio-to-HTTP bridge subcommand (`mcp-bridge`) makes the MCP
  endpoint available as a local CLI tool for AI agents.
- Browser clients can connect to `GET /v1/ws` and receive real-time
  JSON messages when notification state changes.
- The background worker broadcasts state transitions to the WebSocket
  hub after each successful state change.

## Prerequisites

- Step 5 completed: reset endpoint, paginated list endpoint,
  PostgreSQL store, dual-backend dispatcher
- Go 1.26.0 (pinned in `mise.toml`)
- `mise` CLI available on PATH

## New Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/mark3labs/mcp-go` | v0.47.1 | MCP server, tool registration, streamable HTTP transport |
| `github.com/coder/websocket` | v1.8.14 | WebSocket upgrade, read/write with `context.Context` support |

Note: `go mod tidy` resolves exact versions. The versions above are
the targets -- `go mod tidy` may select a compatible patch version.
Run `go mod tidy` during Task 1 when the first new import is added;
this fetches both dependencies early so subsequent tasks compile.

## Spec Traceability

Tasks trace to two specs:

- `openspec/specs/mcp-integration/spec.md` -- REQ-001 through REQ-013
  (MCP server, tools, bridge, graceful shutdown)
- `openspec/specs/notification-realtime/spec.md` -- REQ-001 through
  REQ-015 (WebSocket endpoint, hub pattern, client management,
  broadcast, graceful shutdown)

## Tasks

### Task 1: WebSocket Hub

Satisfies: notification-realtime REQ-004 (hub manages connected
clients), REQ-005 (map of clients with register/unregister/broadcast
channels), REQ-007 (context cancellation exits run loop), REQ-008
(broadcast channel buffered at 256), REQ-012 (slow client dropped
when send channel full), REQ-014 (broadcast delivered to all
clients).

**Files:**
- Create: `internal/infra/ws/hub.go`
- Test: `internal/infra/ws/hub_test.go`

**Step 1: Write the failing test**

Create `internal/infra/ws/hub_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestHub" ./internal/infra/ws/...`
Expected: FAIL with "no Go files in" or "cannot find package"

**Step 3: Write the implementation**

Create `internal/infra/ws/hub.go`:

```go
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
	defer c.Conn.Close(websocket.StatusNormalClosure, "closing")
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
func (c *Client) ReadPump(ctx context.Context) {
	defer func() { c.hub.unregister <- c }()
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
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestHub" ./internal/infra/ws/...`
Expected: PASS (4 tests)

**Step 5: Commit**

`feat(ws): add WebSocket hub with register, unregister, and broadcast`

---

### Task 2: WebSocket HTTP Upgrade Handler

Satisfies: notification-realtime REQ-001 (GET /v1/ws endpoint),
REQ-002 (coder/websocket library), REQ-003 (websocket.Accept),
REQ-009 (dedicated send channel and read/write pump goroutines).

**Depends on:** Task 1 (Hub and Client types)

**Files:**
- Create: `internal/infra/ws/handler.go`
- Test: `internal/infra/ws/handler_test.go`

**Step 1: Write the failing test**

Create `internal/infra/ws/handler_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestHandleWS" ./internal/infra/ws/...`
Expected: FAIL with "undefined: HandleWS"

**Step 3: Write the implementation**

Create `internal/infra/ws/handler.go`:

```go
package ws

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
)

// HandleWS returns an http.HandlerFunc that upgrades HTTP connections
// to WebSocket and registers clients with the hub (REQ-001, REQ-003).
//
// The connCtx parameter provides the lifecycle context for all
// connections. After websocket.Accept hijacks the connection,
// r.Context() is unreliable (it may be cancelled when the HTTP
// handler returns). Use a context derived from the hub's lifecycle
// instead.
func HandleWS(hub *Hub, connCtx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return // Accept writes the HTTP error response.
		}
		client := NewClient(hub, conn)
		hub.Register(client)

		go client.WritePump(connCtx)
		client.ReadPump(connCtx) // blocks until disconnect
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestHandleWS" ./internal/infra/ws/...`
Expected: PASS (4 tests)

**Step 5: Commit**

`feat(ws): add WebSocket HTTP upgrade handler`

---

### Task 3: Domain Port Interfaces

Satisfies: notification-realtime REQ-013 (service layer broadcasts
state changes), mcp-integration REQ-009 (handlers accept domain port
interfaces).

The worker needs to broadcast state changes to the WebSocket hub
after transitions. Rather than importing the `ws` package directly
from the `queue` package (which would couple infra packages), define
a `Broadcaster` port interface in the domain layer. The hub
satisfies it; the worker accepts it.

Additionally, extract `Enqueuer` and `ResetStore` interfaces into
the domain layer so both `httpapi` and `mcp` packages import a
single authoritative definition rather than each declaring their own
identical copies.

**Files:**
- Modify: `internal/domain/store.go`
- Modify: `internal/infra/httpapi/notify.go` (remove local Enqueuer, import from domain)
- Modify: `internal/infra/httpapi/reset.go` (remove local ResetStore, import from domain)
- Test: (no test file -- interface definition only)

**Step 1: Add Broadcaster, Enqueuer, and ResetStore interfaces to the domain ports**

In `internal/domain/store.go`, add after `HealthChecker`:

```go
// Broadcaster pushes real-time state change messages to connected
// clients. Implementations live in infra/ (e.g., WebSocket hub).
type Broadcaster interface {
	Broadcast(msg []byte)
}

// Enqueuer abstracts the job queue. Both httpapi and mcp handlers
// accept this interface rather than a concrete goqite type.
type Enqueuer interface {
	Enqueue(ctx context.Context, payload []byte) error
}

// ResetStore defines the storage interface needed by reset
// operations. It combines notification CRUD, state machine
// accessor/mutator, and transition logging.
type ResetStore interface {
	NotificationStore
	TransitionLogger
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}
```

This requires adding `"github.com/qmuntal/stateless"` to the import
block of `internal/domain/store.go`.

**Step 2: Update httpapi/notify.go to use domain.Enqueuer**

In `internal/infra/httpapi/notify.go`, remove the local `Enqueuer`
interface definition and change `HandleNotify` to accept
`domain.Enqueuer`:

```go
// HandleNotify returns an http.HandlerFunc for POST /v1/notify.
// It validates the email, creates a notification record, and enqueues
// a delivery job. Email sending is asynchronous (REQ-005).
func HandleNotify(store domain.NotificationStore, enqueuer domain.Enqueuer) http.HandlerFunc {
```

**Step 3: Update httpapi/reset.go to use domain.ResetStore**

In `internal/infra/httpapi/reset.go`, remove the local `ResetStore`
interface definition and change `HandleReset` to accept
`domain.ResetStore`:

```go
func HandleReset(store domain.ResetStore) http.HandlerFunc {
```

**Step 4: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 5: Commit**

`feat(domain): add Broadcaster, Enqueuer, and ResetStore port interfaces`

---

### Task 4: Worker Broadcasts State Changes

Satisfies: notification-realtime REQ-013 (broadcast JSON message with
notification id and new state after state changes).

**Depends on:** Task 3 (Broadcaster interface)

**Files:**
- Modify: `internal/infra/queue/worker.go`
- Modify: `internal/infra/queue/worker_test.go` (if exists, add broadcast assertions)

**Step 1: Write the failing test**

Add broadcast verification to the worker test. Create or update
`internal/infra/queue/worker_test.go`:

```go
package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/qmuntal/stateless"

	"github.com/workfort/notifier/internal/domain"
)

// stubBroadcaster captures broadcast messages.
type stubBroadcaster struct {
	messages [][]byte
}

func (b *stubBroadcaster) Broadcast(msg []byte) {
	b.messages = append(b.messages, msg)
}

// stubWorkerStore is a minimal in-memory store for worker tests.
type stubWorkerStore struct {
	notifications map[string]*domain.Notification
	transitions   []string
}

func newStubWorkerStore() *stubWorkerStore {
	return &stubWorkerStore{
		notifications: make(map[string]*domain.Notification),
	}
}

func (s *stubWorkerStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.Email] = n
	return nil
}

func (s *stubWorkerStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (s *stubWorkerStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.Email] = n
	return nil
}

func (s *stubWorkerStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, nil
}

func (s *stubWorkerStore) LogTransition(_ context.Context, _, _ string, from, to domain.Status, _ domain.Trigger) error {
	s.transitions = append(s.transitions, from.String()+"->"+to.String())
	return nil
}

func (s *stubWorkerStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				return n.Status, nil
			}
		}
		return nil, domain.ErrNotFound
	}
}

func (s *stubWorkerStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, state stateless.State) error {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				n.Status = state.(domain.Status)
				return nil
			}
		}
		return domain.ErrNotFound
	}
}

// stubSender is a minimal email sender that always succeeds.
type stubSender struct{}

func (s *stubSender) Send(_ context.Context, _ *domain.EmailMessage) error {
	return nil
}

func TestEmailWorkerBroadcastsOnDelivery(t *testing.T) {
	store := newStubWorkerStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_bcast-1",
		Email:      "user@company.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	bc := &stubBroadcaster{}
	worker := NewEmailWorker(store, &stubSender{}, bc)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_bcast-1",
		Email:          "user@company.com",
		RequestID:      "req_test-1",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Worker should broadcast at least for the sending and delivered
	// transitions.
	if len(bc.messages) < 2 {
		t.Fatalf("broadcasts = %d, want >= 2", len(bc.messages))
	}

	// Verify the last broadcast is the delivered state.
	var last map[string]string
	if err := json.Unmarshal(bc.messages[len(bc.messages)-1], &last); err != nil {
		t.Fatalf("unmarshal last broadcast: %v", err)
	}
	if last["id"] != "ntf_bcast-1" {
		t.Errorf("broadcast id = %q, want ntf_bcast-1", last["id"])
	}
	if last["state"] != "delivered" {
		t.Errorf("broadcast state = %q, want delivered", last["state"])
	}
}

func TestEmailWorkerNilBroadcasterDoesNotPanic(t *testing.T) {
	store := newStubWorkerStore()
	store.notifications["safe@company.com"] = &domain.Notification{
		ID:         "ntf_nil-1",
		Email:      "safe@company.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	// Pass nil broadcaster -- worker should not panic.
	worker := NewEmailWorker(store, &stubSender{}, nil)

	payload, _ := json.Marshal(EmailJobPayload{
		NotificationID: "ntf_nil-1",
		Email:          "safe@company.com",
		RequestID:      "req_test-nil",
	})

	err := worker.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestEmailWorkerBroadcast|TestEmailWorkerNilBroadcaster" ./internal/infra/queue/...`
Expected: FAIL with "too many arguments in call to NewEmailWorker"

**Step 3: Modify the worker to accept a Broadcaster and broadcast after transitions**

Modify `internal/infra/queue/worker.go`:

Change `EmailWorker` struct and `NewEmailWorker` to accept a
`domain.Broadcaster`:

```go
// EmailWorker processes email delivery jobs from the goqite queue.
// It uses the domain state machine for all state transitions and
// logs each transition to the audit log.
type EmailWorker struct {
	store       WorkerStore
	sender      domain.EmailSender
	broadcaster domain.Broadcaster
}

// NewEmailWorker creates a new worker with the given store, email
// sender, and optional broadcaster. If broadcaster is nil, state
// changes are not broadcast (safe for tests and CLI contexts).
func NewEmailWorker(store WorkerStore, sender domain.EmailSender, broadcaster domain.Broadcaster) *EmailWorker {
	return &EmailWorker{store: store, sender: sender, broadcaster: broadcaster}
}
```

Add a helper method to broadcast state changes:

```go
// broadcastState sends a JSON state change message to the WebSocket
// hub. No-op if broadcaster is nil.
func (w *EmailWorker) broadcastState(id string, state domain.Status) {
	if w.broadcaster == nil {
		return
	}
	msg, _ := json.Marshal(map[string]string{
		"id":    id,
		"state": state.String(),
	})
	w.broadcaster.Broadcast(msg)
}
```

Add `w.broadcastState(n.ID, ...)` calls after each successful state
transition in the `Handle` method. Specifically, add broadcast calls
after each `fireAndLog` call that succeeds:

After the send/retry trigger fires (pending/not_sent -> sending):
```go
	if err := w.fireAndLog(ctx, sm, n, prevStatus, sendTrigger, domain.StatusSending); err != nil {
		return err
	}
	w.broadcastState(n.ID, domain.StatusSending)
```

After the delivered trigger fires:
```go
	if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerDelivered, domain.StatusDelivered); fErr != nil {
		slog.Error("fire delivered trigger", "error", fErr)
		return fmt.Errorf("fire delivered: %w", fErr)
	}
	w.broadcastState(n.ID, domain.StatusDelivered)
```

After the failed trigger fires (permanent failure):
```go
	if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
		slog.Error("fire failed trigger", "error", fErr)
	}
	w.broadcastState(n.ID, domain.StatusFailed)
```

After the soft_fail trigger fires (sending -> not_sent):
```go
	// soft_fail succeeded (sending -> not_sent).
	if logErr := w.store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail); logErr != nil {
		slog.Error("log transition failed", "error", logErr)
	}
	w.broadcastState(n.ID, domain.StatusNotSent)
```

After the failed trigger fires (retry limit exhausted):
```go
	if fErr := w.fireAndLog(ctx, sm, n, domain.StatusSending, domain.TriggerFailed, domain.StatusFailed); fErr != nil {
		slog.Error("fire failed trigger after guard rejection", "error", fErr)
	}
	w.broadcastState(n.ID, domain.StatusFailed)
```

**Step 4: Update the daemon to pass the broadcaster to the worker**

This will be done in Task 7 (daemon wiring). For now, the worker
compiles and tests pass.

**Step 5: Run test to verify it passes**

Run: `go test -run "TestEmailWorkerBroadcast|TestEmailWorkerNilBroadcaster" ./internal/infra/queue/...`
Expected: PASS (2 tests)

**Step 6: Update existing worker tests for the new signature**

The existing `internal/infra/queue/worker_test.go` has four call
sites that pass two arguments. Update each to pass `nil` as the
third argument (broadcaster is unused in those tests):

```diff
-	worker := NewEmailWorker(store, sender)
+	worker := NewEmailWorker(store, sender, nil)
```

Apply this change at lines 112, 163, 202, and 244 in
`worker_test.go` (the four `TestEmailWorker*` functions).

Run: `go test ./internal/infra/queue/...`
Expected: PASS (all tests in package, including the four existing
tests and the two new broadcast tests)

**Step 7: Commit**

`feat(queue): broadcast state changes to WebSocket hub after transitions`

---

### Task 5: MCP Tool Handlers

Satisfies: mcp-integration REQ-004 (send_notification tool), REQ-005
(reset_notification tool), REQ-006 (list_notifications tool), REQ-007
(check_health tool), REQ-008 (tool descriptions), REQ-009 (handlers
accept domain port interfaces).

MCP tool handlers call the same service/store layer as REST handlers.
They live in `internal/infra/mcp/` and accept the same domain port
interfaces that the HTTP handlers accept.

**Depends on:** Tasks 1-4 (WebSocket hub and broadcaster for the
send_notification tool's enqueue path -- though MCP tools themselves
do not broadcast, the worker does)

**Files:**
- Create: `internal/infra/mcp/tools.go`
- Test: `internal/infra/mcp/tools_test.go`

**Step 1: Write the failing test**

Create `internal/infra/mcp/tools_test.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/qmuntal/stateless"

	gomcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/workfort/notifier/internal/domain"
)

// --- Stub store for MCP handler tests ---

type stubNotificationStore struct {
	notifications map[string]*domain.Notification
	transitions   []string
}

func newStubStore() *stubNotificationStore {
	return &stubNotificationStore{
		notifications: make(map[string]*domain.Notification),
	}
}

func (s *stubNotificationStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	if _, exists := s.notifications[n.Email]; exists {
		return domain.ErrAlreadyNotified
	}
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (s *stubNotificationStore) UpdateNotification(_ context.Context, n *domain.Notification) error {
	s.notifications[n.Email] = n
	return nil
}

func (s *stubNotificationStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	var list []*domain.Notification
	for _, n := range s.notifications {
		list = append(list, n)
	}
	return list, nil
}

func (s *stubNotificationStore) Ping(_ context.Context) error {
	return nil
}

func (s *stubNotificationStore) LogTransition(_ context.Context, _, _ string, from, to domain.Status, _ domain.Trigger) error {
	s.transitions = append(s.transitions, from.String()+"->"+to.String())
	return nil
}

func (s *stubNotificationStore) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(_ context.Context) (stateless.State, error) {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				return n.Status, nil
			}
		}
		return nil, domain.ErrNotFound
	}
}

func (s *stubNotificationStore) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, state stateless.State) error {
		for _, n := range s.notifications {
			if n.ID == notificationID {
				n.Status = state.(domain.Status)
				return nil
			}
		}
		return domain.ErrNotFound
	}
}

// --- Stub enqueuer ---

type stubEnqueuer struct {
	jobs [][]byte
}

func (e *stubEnqueuer) Enqueue(_ context.Context, payload []byte) error {
	e.jobs = append(e.jobs, payload)
	return nil
}

// --- Tests ---

func TestSendNotificationTool(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should contain the notification ID.
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	// Verify the notification was created.
	if len(store.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(store.notifications))
	}

	// Verify a job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(enqueuer.jobs))
	}
}

func TestSendNotificationToolDuplicate(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		Email: "user@company.com",
	}
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return an error result, not a Go error.
	if result.IsError != true {
		t.Error("expected IsError to be true for duplicate")
	}
}

func TestSendNotificationToolInvalidEmail(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "not-an-email"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError != true {
		t.Error("expected IsError for invalid email")
	}
}

func TestResetNotificationTool(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_mcp-reset-1",
		Email:      "user@company.com",
		Status:     domain.StatusDelivered,
		RetryCount: 2,
		RetryLimit: domain.DefaultRetryLimit,
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError == true {
		t.Error("unexpected error result for reset")
	}

	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", n.Status)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}
}

func TestResetNotificationToolNotFound(t *testing.T) {
	store := newStubStore()
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "nobody@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError != true {
		t.Error("expected IsError for not found")
	}
}

func TestListNotificationsTool(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:     "ntf_list-1",
		Email:  "user@company.com",
		Status: domain.StatusDelivered,
	}
	handler := HandleListNotifications(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	// The result text should be JSON containing the notification.
	text := result.Content[0].(gomcp.TextContent).Text
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	notifications, ok := parsed["notifications"].([]any)
	if !ok || len(notifications) == 0 {
		t.Error("expected notifications array with items")
	}
}

func TestCheckHealthTool(t *testing.T) {
	store := newStubStore()
	handler := HandleCheckHealth(store)

	req := gomcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError == true {
		t.Error("unexpected error result for healthy check")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text == "" {
		t.Error("expected non-empty health result text")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestSendNotification|TestResetNotification|TestListNotifications|TestCheckHealth" ./internal/infra/mcp/...`
Expected: FAIL with "no Go files in" or "cannot find package"

**Step 3: Write the implementation**

Create `internal/infra/mcp/tools.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// HandleSendNotification returns an MCP tool handler for
// send_notification. It calls the same domain logic as POST /v1/notify
// (REQ-004).
func HandleSendNotification(store domain.NotificationStore, enqueuer domain.Enqueuer) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		email := req.GetString("email", "")
		if email == "" {
			return gomcp.NewToolResultError("email is required"), nil
		}

		if err := domain.ValidateEmail(email); err != nil {
			return gomcp.NewToolResultError(err.Error()), nil
		}

		id := domain.NewID("ntf")
		n := &domain.Notification{
			ID:         id,
			Email:      email,
			Status:     domain.StatusPending,
			RetryCount: 0,
			RetryLimit: domain.DefaultRetryLimit,
		}

		if err := store.CreateNotification(ctx, n); err != nil {
			if errors.Is(err, domain.ErrAlreadyNotified) {
				return gomcp.NewToolResultError("already notified"), nil
			}
			return gomcp.NewToolResultError("internal error: " + err.Error()), nil
		}

		jobPayload := queue.EmailJobPayload{
			NotificationID: id,
			Email:          email,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(ctx, payload); err != nil {
			return gomcp.NewToolResultError("failed to enqueue: " + err.Error()), nil
		}

		result, _ := json.Marshal(map[string]string{"id": id})
		return gomcp.NewToolResultText(string(result)), nil
	}
}

// HandleResetNotification returns an MCP tool handler for
// reset_notification. It calls the same domain logic as
// POST /v1/notify/reset (REQ-005).
func HandleResetNotification(store domain.ResetStore) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		email := req.GetString("email", "")
		if email == "" {
			return gomcp.NewToolResultError("email is required"), nil
		}

		n, err := store.GetNotificationByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return gomcp.NewToolResultError("not found"), nil
			}
			return gomcp.NewToolResultError("internal error: " + err.Error()), nil
		}

		prevStatus := n.Status
		sm := domain.ConfigureStateMachine(
			store.NotificationStateAccessor(n.ID),
			store.NotificationStateMutator(n.ID),
			n.RetryLimit,
			n.RetryCount,
		)
		if err := sm.FireCtx(ctx, domain.TriggerReset); err != nil {
			return gomcp.NewToolResultError("reset failed: " + err.Error()), nil
		}

		if err := store.LogTransition(ctx, "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset); err != nil {
			// Log failure is non-fatal.
		}

		n.RetryCount = 0
		n.UpdatedAt = time.Time{}
		if err := store.UpdateNotification(ctx, n); err != nil {
			return gomcp.NewToolResultError("update failed: " + err.Error()), nil
		}

		return gomcp.NewToolResultText("notification reset"), nil
	}
}

// HandleListNotifications returns an MCP tool handler for
// list_notifications. It calls the same domain logic as
// GET /v1/notifications (REQ-006).
func HandleListNotifications(store domain.NotificationStore) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		after := req.GetString("after", "")
		limit := req.GetInt("limit", 20)
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		notifications, err := store.ListNotifications(ctx, after, limit)
		if err != nil {
			return gomcp.NewToolResultError("list failed: " + err.Error()), nil
		}

		type item struct {
			ID         string `json:"id"`
			Email      string `json:"email"`
			State      string `json:"state"`
			RetryCount int    `json:"retry_count"`
			RetryLimit int    `json:"retry_limit"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
		}

		items := make([]item, 0, len(notifications))
		for _, n := range notifications {
			items = append(items, item{
				ID:         n.ID,
				Email:      n.Email,
				State:      n.Status.String(),
				RetryCount: n.RetryCount,
				RetryLimit: n.RetryLimit,
				CreatedAt:  n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt:  n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		result, _ := json.Marshal(map[string]any{
			"notifications": items,
		})
		return gomcp.NewToolResultText(string(result)), nil
	}
}

// HandleCheckHealth returns an MCP tool handler for check_health.
// It calls the same domain logic as GET /v1/health (REQ-007).
func HandleCheckHealth(checker domain.HealthChecker) server.ToolHandlerFunc {
	return func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		err := checker.Ping(ctx)
		status := "healthy"
		if err != nil {
			status = "unhealthy"
		}
		result, _ := json.Marshal(map[string]string{"status": status})
		return gomcp.NewToolResultText(string(result)), nil
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestSendNotification|TestResetNotification|TestListNotifications|TestCheckHealth" ./internal/infra/mcp/...`
Expected: PASS (8 tests)

**Step 5: Commit**

`feat(mcp): add MCP tool handlers for send, reset, list, and health`

---

### Task 6: MCP Server Setup

Satisfies: mcp-integration REQ-001 (MCP endpoint at /mcp), REQ-002
(NewMCPServer with service name and version), REQ-003 (StripPrefix
when mounting on mux), REQ-008 (tool descriptions).

**Depends on:** Task 5 (MCP tool handlers)

**Files:**
- Create: `internal/infra/mcp/server.go`
- Test: `internal/infra/mcp/server_test.go`

**Step 1: Write the failing test**

Create `internal/infra/mcp/server_test.go`:

```go
package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMCPHandlerRespondsToPost(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}

	handler := NewMCPHandler(store, enqueuer, "test")

	// Send an MCP initialize request to verify the handler is wired.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// The MCP server should respond with 200 and a JSON-RPC response.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, string(respBody))
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "jsonrpc") {
		t.Errorf("response does not contain jsonrpc: %s", string(respBody))
	}
}

func TestNewMCPHandlerWithStripPrefix(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}

	mcpHandler := NewMCPHandler(store, enqueuer, "test")

	// Mount with StripPrefix as it will be in production.
	mux := http.NewServeMux()
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		respBody, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, string(respBody))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestNewMCPHandler" ./internal/infra/mcp/...`
Expected: FAIL with "undefined: NewMCPHandler"

**Step 3: Write the implementation**

Create `internal/infra/mcp/server.go`:

```go
package mcp

import (
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/workfort/notifier/internal/domain"
)

// NewMCPHandler creates a StreamableHTTPServer with all MCP tools
// registered. The store must implement all required port interfaces
// (NotificationStore, HealthChecker, TransitionLogger, and the state
// machine accessor/mutator methods). The enqueuer provides job
// queueing. version is the service version string.
//
// Returns the StreamableHTTPServer as an http.Handler. The caller
// must also retain the *server.StreamableHTTPServer to call Shutdown
// during graceful shutdown (REQ-013).
func NewMCPHandler(store domain.ResetStore, enqueuer domain.Enqueuer, version string) *server.StreamableHTTPServer {
	s := server.NewMCPServer("notifier", version)

	s.AddTool(
		gomcp.NewTool("send_notification",
			gomcp.WithDescription("Send a notification email to the given address. Creates a notification record and enqueues an email delivery job."),
			gomcp.WithString("email",
				gomcp.Required(),
				gomcp.Description("Email address to notify"),
			),
		),
		HandleSendNotification(store, enqueuer),
	)

	s.AddTool(
		gomcp.NewTool("reset_notification",
			gomcp.WithDescription("Reset a notification record so the email address can be notified again. Transitions the notification back to pending and clears the retry count."),
			gomcp.WithString("email",
				gomcp.Required(),
				gomcp.Description("Email address of the notification to reset"),
			),
		),
		HandleResetNotification(store),
	)

	s.AddTool(
		gomcp.NewTool("list_notifications",
			gomcp.WithDescription("List all notification records with their current state, retry count, and timestamps."),
			gomcp.WithString("after",
				gomcp.Description("Cursor for pagination (notification ID to start after)"),
			),
			gomcp.WithNumber("limit",
				gomcp.Description("Maximum number of results to return (default 20, max 100)"),
			),
		),
		HandleListNotifications(store),
	)

	s.AddTool(
		gomcp.NewTool("check_health",
			gomcp.WithDescription("Check the health of the notification service by pinging the database."),
		),
		HandleCheckHealth(store),
	)

	return server.NewStreamableHTTPServer(s)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestNewMCPHandler" ./internal/infra/mcp/...`
Expected: PASS (2 tests)

**Step 5: Commit**

`feat(mcp): add MCP server with tool registration and streamable HTTP`

---

### Task 7: Daemon Wiring

Satisfies: notification-realtime REQ-006 (hub Run started before HTTP
server), mcp-integration REQ-001 (MCP endpoint at /mcp), REQ-003
(StripPrefix mounting), REQ-013 (MCP shutdown).

Wire the WebSocket hub, MCP handler, and broadcaster into the daemon.

**Depends on:** Tasks 1-6

**Files:**
- Modify: `cmd/daemon/daemon.go:82-170`

**Step 1: Update imports in daemon.go**

Add the new imports:

```go
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"maragu.dev/goqite"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/db"
	"github.com/workfort/notifier/internal/infra/email"
	"github.com/workfort/notifier/internal/infra/httpapi"
	mcpinfra "github.com/workfort/notifier/internal/infra/mcp"
	"github.com/workfort/notifier/internal/infra/queue"
	"github.com/workfort/notifier/internal/infra/seed"
	"github.com/workfort/notifier/internal/infra/ws"
)
```

**Step 2: Update RunServer to create hub, pass broadcaster to worker, mount WS and MCP**

Replace the section of `RunServer` from the worker creation through
mux setup (approximately lines 124-138) with the updated wiring.

Use separate contexts for the hub and job runner so shutdown ordering
is explicit -- the signal context (`ctx`) triggers the shutdown
sequence, but hub and runner have their own cancellation controlled
by the shutdown block:

```go
	// Create a separate context for the hub. Shutdown cancels it
	// after HTTP connections drain so write pumps can still dispatch
	// messages during graceful shutdown (REQ-015).
	hubCtx, hubCancel := context.WithCancel(context.Background())

	// Create WebSocket hub and start it (REQ-006: before HTTP server).
	hub := ws.NewHub()
	go hub.Run(hubCtx)

	// Create a separate context for the job runner. Shutdown cancels
	// it after the hub so the runner can finish its current job before
	// the store is closed.
	runnerCtx, runnerCancel := context.WithCancel(context.Background())

	// Create and register the email worker with broadcaster.
	worker := queue.NewEmailWorker(store, sender, hub)
	runner := queue.NewJobRunner(nq.Queue())
	runner.Register("send_notification", worker.Handle)

	// Start the job runner in a goroutine.
	go runner.Start(runnerCtx)

	// Create the MCP handler.
	mcpHandler := mcpinfra.NewMCPHandler(store, nq, cli.Version)

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
	mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
	mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))
	mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx))
	mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))
```

**Step 3: Add ordered graceful shutdown**

In the shutdown section of `RunServer`, replace the existing shutdown
block with the proper ordering. The sequence ensures each component
finishes its work before the next is cancelled:

```go
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// 1. Drain HTTP connections -- reject new requests, let
	//    in-flight handlers (including WebSocket upgrades) finish.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}

	// 2. Signal MCP SSE clients to reconnect (REQ-013).
	if err := mcpHandler.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp shutdown", "error", err)
	}

	// 3. Cancel hub context -- hub stops, write pumps close WS
	//    connections with StatusNormalClosure (REQ-015).
	hubCancel()

	// 4. Cancel runner -- job runner drains current job, then exits.
	runnerCancel()

	// 5. Close database (store.Close) happens in the existing defer
	//    block above.
```

**Step 4: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

Note: The hub satisfies `domain.Broadcaster` because it has a
`Broadcast(msg []byte)` method.

**Step 5: Run all existing tests**

Run: `go test ./...`
Expected: PASS (all packages)

**Step 6: Commit**

`feat(daemon): wire WebSocket hub, MCP handler, and broadcaster`

---

### Task 8: MCP Bridge Subcommand

Satisfies: mcp-integration REQ-010 (read JSON-RPC from stdin, forward
to HTTP), REQ-011 (relay responses to stdout), REQ-012 (pass auth
token on requests).

The bridge is a simple stdin/stdout relay: it reads newline-delimited
JSON-RPC messages from stdin, POSTs them to `http://<host>:<port>/mcp`,
and writes responses to stdout. This lets AI agents that speak stdio
MCP connect to the HTTP service.

**Depends on:** Task 6 (MCP server)

**Files:**
- Create: `cmd/mcp-bridge/bridge.go`
- Create: `cmd/mcp-bridge/bridge_test.go`

**Step 1: Write the failing test**

Create `cmd/mcp-bridge/bridge_test.go`:

```go
package mcpbridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridgeForwardsRequest(t *testing.T) {
	// Create a fake MCP server that echoes back a JSON-RPC response.
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request is a POST with JSON content.
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}
		// Verify auth token is passed.
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("authorization = %q, want 'Bearer test-token'", auth)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		// Return a JSON-RPC response.
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]string{"status": "ok"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeMCP.Close()

	// Simulate stdin with a JSON-RPC request.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_health"}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	err := Bridge(stdin, &stdout, fakeMCP.URL, "test-token")
	if err != nil {
		t.Fatalf("Bridge error: %v", err)
	}

	// Verify stdout contains the response.
	output := strings.TrimSpace(stdout.String())
	if !strings.Contains(output, "jsonrpc") {
		t.Errorf("stdout = %q, want JSON-RPC response", output)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["id"] != float64(1) {
		t.Errorf("id = %v, want 1", resp["id"])
	}
}

func TestBridgeHandlesMultipleMessages(t *testing.T) {
	var receivedCount int
	fakeMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]string{"status": "ok"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeMCP.Close()

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"check_health"}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	err := Bridge(stdin, &stdout, fakeMCP.URL, "")
	if err != nil {
		t.Fatalf("Bridge error: %v", err)
	}

	if receivedCount != 2 {
		t.Errorf("received = %d, want 2", receivedCount)
	}

	// Verify two responses were written.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("response lines = %d, want 2", len(lines))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run "TestBridge" ./cmd/mcp-bridge/...`
Expected: FAIL with "undefined: Bridge"

**Step 3: Write the implementation**

Replace the contents of `cmd/mcp-bridge/bridge.go` (which currently
contains only a `doc.go` package declaration):

```go
package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
)

// NewCmd creates the mcp-bridge subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-bridge",
		Short: "Stdio-to-HTTP MCP bridge",
		Long:  "Reads JSON-RPC messages from stdin, forwards them to the MCP HTTP endpoint, and relays responses to stdout.",
		RunE:  runBridge,
	}
	cmd.Flags().String("url", "http://127.0.0.1:8080/mcp", "MCP endpoint URL")
	cmd.Flags().String("token", "", "Auth token for MCP requests")
	return cmd
}

func runBridge(cmd *cobra.Command, _ []string) error {
	url := resolveString(cmd, "url")
	token := resolveString(cmd, "token")
	return Bridge(os.Stdin, os.Stdout, url, token)
}

// Bridge reads newline-delimited JSON-RPC messages from r, POSTs each
// to the given URL, and writes responses to w. Each line of input is
// one JSON-RPC message (REQ-010, REQ-011).
func Bridge(r io.Reader, w io.Writer, url string, token string) error {
	client := &http.Client{}
	scanner := bufio.NewScanner(r)
	// Allow up to 1 MB per line.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			url,
			bytes.NewReader(line),
		)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// REQ-012: pass auth token on every request.
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			slog.Error("forward request", "error", err)
			continue // skip failed requests, don't crash the bridge
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			slog.Error("read response", "error", err)
			continue
		}

		// Write response to stdout, one line per response.
		fmt.Fprintf(w, "%s\n", bytes.TrimSpace(body))
	}

	return scanner.Err()
}

// resolveString reads from koanf if the key exists, otherwise from
// the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.String(dotKey)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestBridge" ./cmd/mcp-bridge/...`
Expected: PASS (2 tests)

**Step 5: Register the mcp-bridge subcommand in the CLI root**

Modify `internal/cli/root.go` to add the mcp-bridge subcommand:

```go
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/cmd/daemon"
	mcpbridge "github.com/workfort/notifier/cmd/mcp-bridge"
	"github.com/workfort/notifier/internal/config"
)

// Version is set from main.go, which receives it via -ldflags.
var Version = "dev"

// NewRootCmd creates the root cobra command with PersistentPreRunE
// that initialises XDG directories and loads configuration.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notifier",
		Short:   "Notification service",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := config.InitDirs(); err != nil {
				return err
			}
			return config.Load()
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(daemon.NewCmd())
	cmd.AddCommand(mcpbridge.NewCmd())
	return cmd
}

// Execute runs the root command. Exits with code 1 on error.
func Execute() {
	root := NewRootCmd()
	root.Version = Version
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 6: Delete the old doc.go**

The `cmd/mcp-bridge/doc.go` placeholder is superseded by
`cmd/mcp-bridge/bridge.go` which declares the same package. Delete
`cmd/mcp-bridge/doc.go`.

**Step 7: Verify the project compiles**

Run: `go build ./...`
Expected: exits 0

**Step 8: Commit**

`feat(mcp-bridge): add stdio-to-HTTP MCP bridge subcommand`

---

### Task 9: Verify Dependencies in go.mod

Dependencies were added via `go mod tidy` during Task 1 (see
Prerequisites / New Dependencies). This task verifies the final
state.

**Files:**
- Verify: `go.mod`
- Verify: `go.sum`

**Step 1: Run go mod tidy**

Run: `go mod tidy`
Expected: exits 0. `go.mod` gains:
- `github.com/mark3labs/mcp-go v0.47.1` (or compatible)
- `github.com/coder/websocket v1.8.14` (or compatible)

**Step 2: Verify no unexpected dependencies were added**

Run: `go mod tidy && go build ./...`
Expected: exits 0

**Step 3: Commit**

`chore(deps): add mcp-go v0.47.1 and coder/websocket v1.8.14`

---

### Task 10: Integration Smoke Test

Satisfies: verification that all capabilities work end-to-end.

**Files:**
- No new files -- uses existing test infrastructure

**Step 1: Run the full test suite**

Run: `go test ./...`
Expected: PASS (all packages)

**Step 2: Run the QA build test suite**

Run: `go build -tags qa ./...`
Expected: exits 0 (compiles with no errors)

Run: `go test -tags qa ./...`
Expected: PASS (all packages)

**Step 3: Run linter**

Run: `mise run lint:go`
Expected: exits 0 (no warnings)

**Step 4: Commit** (no commit needed -- verification only)

## Verification Checklist

- [ ] `go build ./...` succeeds with no warnings
- [ ] `go build -tags qa ./...` succeeds with no warnings
- [ ] `go test ./...` passes (all packages)
- [ ] `go test -tags qa ./...` passes (all packages)
- [ ] `mise run lint:go` produces no warnings
- [ ] WebSocket hub registers clients and broadcasts messages (REQ-004, REQ-014)
- [ ] WebSocket hub drops slow clients whose send channel is full (REQ-012)
- [ ] WebSocket hub exits when context is cancelled (REQ-007)
- [ ] `GET /v1/ws` upgrades to WebSocket and receives broadcast messages (REQ-001, REQ-003)
- [ ] Multiple WebSocket clients receive the same broadcast (scenario: multiple clients)
- [ ] Disconnected WebSocket client is cleaned up by read pump (REQ-011)
- [ ] Worker broadcasts state changes after each transition (REQ-013)
- [ ] Worker with nil broadcaster does not panic
- [ ] MCP `send_notification` tool creates notification and enqueues job (REQ-004)
- [ ] MCP `send_notification` with duplicate email returns error result (scenario: duplicate prevention)
- [ ] MCP `send_notification` with invalid email returns error result
- [ ] MCP `reset_notification` tool resets notification to pending (REQ-005)
- [ ] MCP `reset_notification` with unknown email returns error result
- [ ] MCP `list_notifications` tool returns JSON with notifications array (REQ-006)
- [ ] MCP `check_health` tool returns healthy/unhealthy status (REQ-007)
- [ ] MCP tools accept domain port interfaces, not concrete types (REQ-009)
- [ ] MCP endpoint at `/mcp` responds to JSON-RPC initialize (REQ-001)
- [ ] MCP StripPrefix mounting works (REQ-003)
- [ ] MCP StreamableHTTPServer shutdown called during graceful shutdown (REQ-013)
- [ ] MCP bridge reads JSON-RPC from stdin and forwards to HTTP (REQ-010)
- [ ] MCP bridge relays HTTP responses to stdout (REQ-011)
- [ ] MCP bridge passes auth token header (REQ-012)
- [ ] MCP bridge handles multiple sequential messages
- [ ] `notifier mcp-bridge --help` shows usage
- [ ] Hub started as goroutine in daemon RunE before HTTP server (REQ-006)
- [ ] Compile-time check: hub satisfies `domain.Broadcaster` interface
