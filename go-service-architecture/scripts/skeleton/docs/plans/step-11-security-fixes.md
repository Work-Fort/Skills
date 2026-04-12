---
type: plan
step: "11"
title: "Security Fixes"
status: pending
assessment_status: in_progress
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "11"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-3-notification-delivery
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
---

# Step 11: Security Fixes

## Overview

Hardens four security gaps identified in future work items #10, #12,
#13, and #14. MCP tool handlers currently expose raw `err.Error()`
strings to clients, leaking database driver details, file paths, and
connection strings. The WebSocket read pump has no frame size limit,
the hub accepts unlimited connections, and the POST handlers accept
unbounded request bodies. All four are addressed in this step.

Deliverables:

1. **MCP error sanitization** -- MCP tools log the real error
   server-side via `slog.Error` and return a generic `"internal error"`
   to the client. Domain-safe errors (`"email is required"`,
   `"already notified"`, `"not found"`, validation errors) remain
   as-is. Satisfies mcp-integration REQ-014, REQ-015, REQ-016.

2. **WebSocket read limit** -- `ReadPump` calls
   `conn.SetReadLimit(512)` before entering the read loop. Satisfies
   notification-realtime REQ-016.

3. **WebSocket connection limit** -- `NewHub(maxConns int)` enforces
   a configurable maximum. The hub rejects registrations above the
   limit by closing the client's send channel. Default 1000 in the
   daemon. Satisfies notification-realtime REQ-017, REQ-018, REQ-019.

4. **MaxBytesReader on POST endpoints** -- `POST /v1/notify` and
   `POST /v1/notify/reset` apply `http.MaxBytesReader(w, r.Body,
   1<<20)` before reading the body. Oversized bodies fall through to
   the existing `"invalid JSON body"` decode error path (HTTP 400).
   Satisfies notification-delivery REQ-030 and
   notification-management REQ-018.

## Prerequisites

- Step 3 completed: `HandleNotify` exists in
  `internal/infra/httpapi/notify.go`.
- Step 5 completed: `HandleReset` exists in
  `internal/infra/httpapi/reset.go`.
- Step 6 completed: MCP tools exist in `internal/infra/mcp/tools.go`,
  WebSocket hub and handler exist in `internal/infra/ws/hub.go` and
  `internal/infra/ws/handler.go`.

## Tasks

### Task 1: MCP error sanitization

**Files:**
- Modify: `internal/infra/mcp/tools.go:39-52` (HandleSendNotification)
- Modify: `internal/infra/mcp/tools.go:71-97` (HandleResetNotification)
- Modify: `internal/infra/mcp/tools.go:115-117` (HandleListNotifications)
- Test: `internal/infra/mcp/tools_test.go`

**Step 1: Write failing tests for error sanitization**

Add the following tests to `internal/infra/mcp/tools_test.go`. These
require a stub store that returns internal errors, so first add a
`failStore` type and then the test functions.

Add the `failStore` type and the `failEnqueuer` type after the
existing `stubEnqueuer` type (after line 98):

```go
// --- Fail store for error-leakage tests ---

type failStore struct {
	stubNotificationStore
}

func (s *failStore) CreateNotification(_ context.Context, _ *domain.Notification) error {
	return fmt.Errorf("pq: connection refused to 10.0.0.5:5432")
}

func (s *failStore) GetNotificationByEmail(_ context.Context, _ string) (*domain.Notification, error) {
	return nil, fmt.Errorf("pq: connection refused to 10.0.0.5:5432")
}

func (s *failStore) ListNotifications(_ context.Context, _ string, _ int) ([]*domain.Notification, error) {
	return nil, fmt.Errorf("pq: relation \"notifications\" does not exist")
}

// --- Fail enqueuer for error-leakage tests ---

type failEnqueuer struct{}

func (e *failEnqueuer) Enqueue(_ context.Context, _ []byte) error {
	return fmt.Errorf("goqite: queue full, 10.0.0.5:5432 not responding")
}

// --- Fail store for state machine error path (tools.go line 86) ---

type failStateMachineStore struct {
	stubNotificationStore
}

func (s *failStateMachineStore) NotificationStateMutator(_ string) func(ctx context.Context, state stateless.State) error {
	return func(_ context.Context, _ stateless.State) error {
		return fmt.Errorf("pq: could not serialize access due to concurrent update on 10.0.0.5:5432")
	}
}

// --- Fail store for update error path (tools.go line 96) ---

type failUpdateStore struct {
	stubNotificationStore
}

func (s *failUpdateStore) UpdateNotification(_ context.Context, _ *domain.Notification) error {
	return fmt.Errorf("pq: deadlock detected on 10.0.0.5:5432")
}
```

Add `"fmt"`, `"strings"`, and `"github.com/qmuntal/stateless"` to the
test file's imports.

Then add the test functions at the end of the file:

```go
func TestSendNotificationToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	enqueuer := &stubEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestSendNotificationToolEnqueueErrorSanitized(t *testing.T) {
	store := newStubStore()
	enqueuer := &failEnqueuer{}
	handler := HandleSendNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for enqueue failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "goqite") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestResetNotificationToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestListNotificationsToolInternalErrorSanitized(t *testing.T) {
	store := &failStore{}
	handler := HandleListNotifications(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for internal failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "relation") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestResetNotificationToolStateMachineErrorSanitized(t *testing.T) {
	store := &failStateMachineStore{
		stubNotificationStore: stubNotificationStore{
			notifications: map[string]*domain.Notification{
				"user@company.com": {
					ID:         "ntf_sm",
					Email:      "user@company.com",
					Status:     domain.StatusDelivered,
					RetryCount: 0,
					RetryLimit: domain.DefaultRetryLimit,
				},
			},
		},
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for state machine failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") || strings.Contains(text, "serialize") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}

func TestResetNotificationToolUpdateErrorSanitized(t *testing.T) {
	store := &failUpdateStore{
		stubNotificationStore: stubNotificationStore{
			notifications: map[string]*domain.Notification{
				"user@company.com": {
					ID:         "ntf_upd",
					Email:      "user@company.com",
					Status:     domain.StatusDelivered,
					RetryCount: 0,
					RetryLimit: domain.DefaultRetryLimit,
				},
			},
		},
	}
	handler := HandleResetNotification(store)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for update failure")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "internal error" {
		t.Errorf("error text = %q, want %q", text, "internal error")
	}
	if strings.Contains(text, "pq:") || strings.Contains(text, "10.0.0.5") || strings.Contains(text, "deadlock") {
		t.Errorf("error text leaks internal details: %q", text)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestSendNotificationToolInternalErrorSanitized|TestSendNotificationToolEnqueueErrorSanitized|TestResetNotificationToolInternalErrorSanitized|TestResetNotificationToolStateMachineErrorSanitized|TestResetNotificationToolUpdateErrorSanitized|TestListNotificationsToolInternalErrorSanitized" ./internal/infra/mcp/...`

Expected: FAIL. The tests will fail because the current code returns
`"internal error: pq: connection refused to 10.0.0.5:5432"` (or
similar internal strings) instead of the generic `"internal error"`.

**Step 3: Sanitize error responses in tools.go**

Add `"log/slog"` to the imports in `internal/infra/mcp/tools.go`.

In `HandleSendNotification`, replace lines 43-44:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("internal error: " + err.Error()), nil

			slog.Error("create notification failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
```

Replace lines 51-52:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("failed to enqueue: " + err.Error()), nil

			slog.Error("enqueue notification job failed",
				"error", err,
				"notification_id", id,
			)
			return gomcp.NewToolResultError("internal error"), nil
```

In `HandleResetNotification`, replace lines 75-76:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("internal error: " + err.Error()), nil

			slog.Error("get notification for reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
```

Replace lines 86-87:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("reset failed: " + err.Error()), nil

			slog.Error("state machine reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
```

Replace lines 96-97:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("update failed: " + err.Error()), nil

			slog.Error("update notification for reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
```

In `HandleListNotifications`, replace lines 116-117:

```go
			// Before (leaks internal details):
			// return gomcp.NewToolResultError("list failed: " + err.Error()), nil

			slog.Error("list notifications failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
```

The full set of changes to `internal/infra/mcp/tools.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
			slog.Error("create notification failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
		}

		jobPayload := queue.EmailJobPayload{
			NotificationID: id,
			Email:          email,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(ctx, payload); err != nil {
			slog.Error("enqueue notification job failed",
				"error", err,
				"notification_id", id,
			)
			return gomcp.NewToolResultError("internal error"), nil
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
			slog.Error("get notification for reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
		}

		prevStatus := n.Status
		sm := domain.ConfigureStateMachine(
			store.NotificationStateAccessor(n.ID),
			store.NotificationStateMutator(n.ID),
			n.RetryLimit,
			n.RetryCount,
		)
		if err := sm.FireCtx(ctx, domain.TriggerReset); err != nil {
			slog.Error("state machine reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
		}

		//nolint:errcheck // Log failure is non-fatal; reset already succeeded.
		_ = store.LogTransition(ctx, "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset)

		n.RetryCount = 0
		n.UpdatedAt = time.Time{}
		if err := store.UpdateNotification(ctx, n); err != nil {
			slog.Error("update notification for reset failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
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
			slog.Error("list notifications failed", "error", err)
			return gomcp.NewToolResultError("internal error"), nil
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

Satisfies: mcp-integration REQ-014, REQ-015, REQ-016.

**Step 4: Run tests to verify they pass**

Run: `go test -run "TestSendNotificationToolInternalErrorSanitized|TestSendNotificationToolEnqueueErrorSanitized|TestResetNotificationToolInternalErrorSanitized|TestResetNotificationToolStateMachineErrorSanitized|TestResetNotificationToolUpdateErrorSanitized|TestListNotificationsToolInternalErrorSanitized" ./internal/infra/mcp/...`

Expected: PASS

**Step 5: Run all MCP tests to verify no regressions**

Run: `go test ./internal/infra/mcp/...`

Expected: PASS. Existing tests (`TestSendNotificationTool`,
`TestSendNotificationToolDuplicate`, `TestResetNotificationTool`,
`TestResetNotificationToolNotFound`, `TestListNotificationsTool`,
`TestCheckHealthTool`) still pass because domain-safe errors
(`"email is required"`, `"already notified"`, `"not found"`) are
unchanged.

**Step 6: Commit**

`fix(mcp): sanitize internal error responses to prevent information leakage`

### Task 2: WebSocket read limit

**Files:**
- Modify: `internal/infra/ws/hub.go:51-58` (ReadPump)
- Test: `internal/infra/ws/hub_test.go`

**Step 1: Write a failing test for read limit**

Add the following test to `internal/infra/ws/hub_test.go`. This test
verifies that the read pump sets a read limit by confirming the
connection is closed after receiving an oversized message.

Add `"net/http/httptest"`, `"strings"`, and `"github.com/coder/websocket"`
to the test file imports (these are already imported in
`handler_test.go` but not in `hub_test.go`).

```go
func TestReadPumpRejectsLargeFrame(t *testing.T) {
	hub := NewHub(1000)
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
```

**Step 2: Run the test to verify it fails**

Run: `go test -run TestReadPumpRejectsLargeFrame ./internal/infra/ws/...`

Expected: FAIL. The test will fail because the server currently
accepts the large frame (no read limit set) and the client can still
read without error from its perspective, or the test may hang waiting
for an error that never comes. The exact failure depends on timing,
but the point is: without `SetReadLimit`, the server does not close
the connection after a large frame.

**Step 3: Add `conn.SetReadLimit(512)` to ReadPump**

In `internal/infra/ws/hub.go`, modify the `ReadPump` method. Add the
`SetReadLimit` call before the read loop:

```go
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
```

Satisfies: notification-realtime REQ-016.

**Step 4: Run the test to verify it passes**

Run: `go test -run TestReadPumpRejectsLargeFrame ./internal/infra/ws/...`

Expected: PASS

**Step 5: Run all WebSocket tests to verify no regressions**

Run: `go test ./internal/infra/ws/...`

Expected: PASS. Existing tests still pass because they do not send
messages larger than 512 bytes from the client.

**Step 6: Commit**

`fix(ws): set 512-byte read limit on WebSocket connections`

### Task 3: WebSocket connection limit

**Depends on:** Task 2 (hub signature changes)

**Files:**
- Modify: `internal/infra/ws/hub.go:62-78` (Hub struct, NewHub)
- Modify: `internal/infra/ws/hub.go:96-120` (Run)
- Modify: `internal/infra/ws/handler.go` (update NewHub call in tests)
- Modify: `internal/infra/ws/handler_test.go` (update NewHub calls)
- Modify: `internal/infra/ws/hub_test.go` (update NewHub calls)
- Modify: `cmd/daemon/daemon.go:155` (pass maxConns)
- Test: `internal/infra/ws/hub_test.go`

**Step 1: Write failing tests for connection limit**

Add the following tests to `internal/infra/ws/hub_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestHubRejectsRegistrationAtCapacity|TestHubAcceptsBelowCapacity|TestHubAcceptsAfterUnregister" ./internal/infra/ws/...`

Expected: FAIL. Compilation error because `NewHub` does not accept a
`maxConns` argument yet.

**Step 3: Add maxConns to Hub and enforce in Run**

Modify `internal/infra/ws/hub.go`. The full updated Hub struct,
constructor, and Run method:

```go
// Hub manages all connected WebSocket clients and broadcasts
// messages to them (REQ-004).
type Hub struct {
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	maxConns   int
}

// NewHub creates a hub with a buffered broadcast channel (REQ-008).
// maxConns sets the maximum number of concurrent client connections.
// Registrations above this limit are rejected by closing the client's
// send channel (REQ-017, REQ-018, REQ-019).
func NewHub(maxConns int) *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		maxConns:   maxConns,
	}
}
```

Update the `Run` method to enforce the connection limit on
registration:

```go
// Run processes register, unregister, and broadcast events until the
// context is cancelled (REQ-006, REQ-007).
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.register:
			if len(h.clients) >= h.maxConns {
				// At capacity -- reject the registration by closing
				// the send channel. WritePump will detect the closed
				// channel and close the underlying connection, which
				// causes ReadPump to exit (REQ-018).
				close(c.send)
			} else {
				h.clients[c] = struct{}{}
			}
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

Satisfies: notification-realtime REQ-017, REQ-018, REQ-019.

**Step 4: Update existing NewHub calls in hub_test.go**

Update all existing `NewHub()` calls in `internal/infra/ws/hub_test.go`
to `NewHub(1000)`:

- `TestHubRegisterAndBroadcast`: `NewHub()` -> `NewHub(1000)`
- `TestHubUnregister`: `NewHub()` -> `NewHub(1000)`
- `TestHubDropsSlowClient`: `NewHub()` -> `NewHub(1000)`
- `TestHubContextCancellation`: `NewHub()` -> `NewHub(1000)`

**Step 5: Update existing NewHub calls in handler_test.go**

Update all existing `NewHub()` calls in
`internal/infra/ws/handler_test.go` to `NewHub(1000)`:

- `TestHandleWSAcceptsConnection`: `NewHub()` -> `NewHub(1000)`
- `TestHandleWSClientDisconnect`: `NewHub()` -> `NewHub(1000)`
- `TestHandleWSMultipleClients`: `NewHub()` -> `NewHub(1000)`
- `TestHandleWSNonUpgradeRequest`: `NewHub()` -> `NewHub(1000)`

**Step 6: Update daemon.go to pass maxConns**

In `cmd/daemon/daemon.go`, change line 155 from:

```go
	hub := ws.NewHub()
```

to:

```go
	hub := ws.NewHub(1000)
```

**Step 7: Run the new connection limit tests**

Run: `go test -run "TestHubRejectsRegistrationAtCapacity|TestHubAcceptsBelowCapacity|TestHubAcceptsAfterUnregister" ./internal/infra/ws/...`

Expected: PASS

**Step 8: Run all WebSocket tests to verify no regressions**

Run: `go test ./internal/infra/ws/...`

Expected: PASS

**Step 9: Commit**

`fix(ws): enforce configurable connection limit in hub`

### Task 4: MaxBytesReader on POST endpoints

**Files:**
- Modify: `internal/infra/httpapi/notify.go:27-29`
- Modify: `internal/infra/httpapi/reset.go:23-26`
- Test: `internal/infra/httpapi/notify_test.go`
- Test: `internal/infra/httpapi/reset_test.go`

**Step 1: Write failing tests for oversized request bodies**

Locate the existing test files for the notify and reset handlers. Add
oversized body tests. These tests send a body larger than 1 MB and
verify the handler returns 400.

Add to `internal/infra/httpapi/notify_test.go`:

```go
func TestHandleNotifyOversizedBody(t *testing.T) {
	store := newStubStore()
	enqueuer := &stubEnqueuer{}
	handler := HandleNotify(store, enqueuer)

	// Create a body larger than 1 MB.
	body := make([]byte, 1<<20+1)
	for i := range body {
		body[i] = 'a'
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
```

Add `"bytes"` to the test file imports if not already present.

Add to `internal/infra/httpapi/reset_test.go`:

```go
func TestHandleResetOversizedBody(t *testing.T) {
	store := newResetStubStore()
	handler := HandleReset(store)

	// Create a body larger than 1 MB.
	body := make([]byte, 1<<20+1)
	for i := range body {
		body[i] = 'a'
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
```

Add `"bytes"` to the test file imports if not already present.

**Step 2: Run tests to verify they fail**

Run: `go test -run "TestHandleNotifyOversizedBody|TestHandleResetOversizedBody" ./internal/infra/httpapi/...`

Expected: FAIL. Without `MaxBytesReader`, the handler will attempt to
decode the oversized body. The exact failure depends on whether the
body parses as JSON (it won't -- it's all `a` characters), so the
handler returns 400 but only after reading the entire body into
memory. The test may actually pass on the status code but the
protection is not in place. If the test passes, that confirms the
existing decode error path handles it, but we still need
`MaxBytesReader` to prevent the server from reading the full body
into memory.

Note: if the test already returns 400 because the all-`a` body fails
JSON decode, that is expected. The `MaxBytesReader` addition prevents
the server from buffering the full body before discovering the error.
The test verifies the end-to-end behavior is correct regardless.

**Step 3: Add MaxBytesReader to HandleNotify**

In `internal/infra/httpapi/notify.go`, add the `MaxBytesReader` call
as the first line inside the returned handler function, before the
JSON decode:

```go
func HandleNotify(store domain.NotificationStore, enqueuer domain.Enqueuer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit (REQ-030)

		var req notifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
```

The rest of the function remains unchanged. The `MaxBytesReader`
causes `json.Decode` to fail with a `MaxBytesError` when the limit
is exceeded, which falls through to the existing `"invalid JSON body"`
error path returning 400.

Satisfies: notification-delivery REQ-030.

**Step 4: Add MaxBytesReader to HandleReset**

In `internal/infra/httpapi/reset.go`, add the `MaxBytesReader` call
as the first line inside the returned handler function:

```go
func HandleReset(store domain.ResetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit (REQ-018)

		var req resetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
```

The rest of the function remains unchanged.

Satisfies: notification-management REQ-018.

**Step 5: Run the oversized body tests**

Run: `go test -run "TestHandleNotifyOversizedBody|TestHandleResetOversizedBody" ./internal/infra/httpapi/...`

Expected: PASS

**Step 6: Run all HTTP handler tests to verify no regressions**

Run: `go test ./internal/infra/httpapi/...`

Expected: PASS

**Step 7: Commit**

`fix(httpapi): add MaxBytesReader to POST endpoints to limit body size`

### Task 5: Update future-work.md

**Files:**
- Modify: `docs/future-work.md`

**Step 1: Mark future work items #10, #12, #13, #14 as resolved**

Change `## #10 — Security: MCP Error Information Leakage (HIGH)` to
`## #10 — Security: MCP Error Information Leakage (HIGH) ✅`

Change `## #12 — Security: WebSocket Read Size Limit (HIGH)` to
`## #12 — Security: WebSocket Read Size Limit (HIGH) ✅`

Change `## #13 — Security: Request Body Size Limits (MEDIUM)` to
`## #13 — Security: Request Body Size Limits (MEDIUM) ✅`

Change `## #14 — Security: WebSocket Connection Limit (MEDIUM)` to
`## #14 — Security: WebSocket Connection Limit (MEDIUM) ✅`

Item #11 (WebSocket Origin Validation) is NOT resolved in this step
and should remain unmarked.

**Step 2: Commit**

`docs: mark future work items #10, #12, #13, #14 as resolved`

### Task 6: Full verification

**Depends on:** Task 1, Task 2, Task 3, Task 4, Task 5

**Step 1: Run the full test suite**

Run: `mise run test:unit`

Expected: PASS. All tests pass.

**Step 2: Build the project**

Run: `mise run build:go`

Expected: BUILD SUCCESS with no warnings.

**Step 3: Run the linter**

Run: `mise run lint:go`

Expected: no warnings or errors.

**Step 4: Verify with QA tag**

Run: `go test -tags qa ./...`

Expected: PASS. The `NewHub(1000)` change and `MaxBytesReader`
additions do not conflict with QA build tags.

## Verification Checklist

- [ ] `internal/infra/mcp/tools.go` never returns `err.Error()` to
      clients. All internal error paths use `slog.Error` + generic
      `"internal error"` (mcp-integration REQ-014, REQ-015)
- [ ] Domain-safe errors (`"email is required"`, `"already notified"`,
      `"not found"`, validation errors) are still returned as-is
      (mcp-integration REQ-016)
- [ ] `internal/infra/mcp/tools.go` imports `"log/slog"`
- [ ] `ReadPump` calls `c.Conn.SetReadLimit(512)` before the read
      loop (notification-realtime REQ-016)
- [ ] `Hub` struct has a `maxConns int` field
- [ ] `NewHub` accepts `maxConns int` parameter
      (notification-realtime REQ-019)
- [ ] `Hub.Run` rejects registrations when `len(h.clients) >= h.maxConns`
      by closing the client's send channel
      (notification-realtime REQ-017, REQ-018)
- [ ] `cmd/daemon/daemon.go` calls `ws.NewHub(1000)`
- [ ] All existing `NewHub()` calls in test files updated to
      `NewHub(1000)`
- [ ] `HandleNotify` applies `http.MaxBytesReader(w, r.Body, 1<<20)`
      before reading the body (notification-delivery REQ-030)
- [ ] `HandleReset` applies `http.MaxBytesReader(w, r.Body, 1<<20)`
      before reading the body (notification-management REQ-018)
- [ ] Oversized body tests pass for both endpoints
- [ ] `mise run test:unit` passes
- [ ] `mise run build:go` succeeds
- [ ] `mise run lint:go` passes with no warnings
- [ ] `go test -tags qa ./...` passes
- [ ] Future work items #10, #12, #13, #14 marked resolved
- [ ] Future work item #11 (origin validation) remains unmarked
