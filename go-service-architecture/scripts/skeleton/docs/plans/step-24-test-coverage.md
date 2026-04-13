---
type: plan
step: "24"
title: "Expand test coverage across 9 gaps"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "24"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-4-state-machine
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
  - step-12-websocket-origin
  - step-19-resend-ux-reset-guard
---

# Step 24: Expand test coverage across 9 gaps

## Overview

The service has solid unit tests and a handful of E2E tests, but nine
specific coverage gaps remain across four specs. This plan adds the
missing tests without changing any production code. The gaps fall into
two categories:

**E2E tests** (in `tests/e2e/`):
1. WebSocket broadcast on state transitions (notification-realtime REQ-024, REQ-025)
2. Pagination edge cases: limit clamping, empty list, total_pages (notification-management REQ-028)
3. Health endpoint dedicated test (notification-management REQ-026)
4. Request ID propagated to Mailpit email headers (notification-delivery REQ-031)

**Integration tests** (in `internal/infra/`):
5. Retry lifecycle with real SQLite + mock sender (notification-state-machine REQ-026)
6. Worker processes job with real SQLite store, creates audit log (notification-state-machine REQ-028)
7. Queue enqueue-dequeue-process cycle with real goqite (notification-delivery REQ-032)
8. Retry guard rejection with real database (notification-management REQ-029)
9. Reset guard 409 with real database: not_sent + retries remaining (notification-management REQ-029)

**Resolved ambiguities applied:**
- Gaps 5, 8, 9: integration tests with real SQLite + mock sender (no
  QA sender in E2E builds).
- Gap 2: pagination clamping is silent -- E2E sends `limit=200`,
  verifies at most 100 returned.
- Gap 7: uses `goqite.Queue.Receive()` directly.
- Gap 1: uses `coder/websocket` client in E2E test.
- Gaps 6, 8, 9: placed alongside `statemachine_integration_test.go`.
- Gap 7: placed in `internal/infra/queue/`.

## Prerequisites

- All steps through 22 are complete and tests pass.
- Mailpit is running on `127.0.0.1:1025` (SMTP) / `127.0.0.1:8025`
  (API) for E2E tests.
- The `coder/websocket` library is already in the main module's
  `go.mod` but must be added to `tests/e2e/go.mod` for the WebSocket
  E2E test.

## Tasks

### Task 1: Add `coder/websocket` dependency to E2E test module

**Files:**
- Modify: `tests/e2e/go.mod`

The WebSocket E2E test (Task 2) imports `github.com/coder/websocket`.
The E2E module is a separate Go module with its own `go.mod`.

**Step 1: Add the dependency**

```
cd tests/e2e && go get github.com/coder/websocket@v1.8.14
```

This updates `tests/e2e/go.mod` and creates/updates `tests/e2e/go.sum`.

**Step 2: Verify the module resolves**

Run: `cd tests/e2e && go mod tidy`
Expected: No errors. `go.mod` lists `github.com/coder/websocket v1.8.14`.

**Step 3: Commit**

`chore(e2e): add coder/websocket dependency for WebSocket E2E test`

### Task 2: E2E test -- WebSocket broadcast on state transitions

**Depends on:** Task 1

**Files:**
- Create: `tests/e2e/websocket_test.go`

Satisfies notification-realtime REQ-024, REQ-025.

**Step 1: Write the test**

```go
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestWebSocketBroadcast verifies that connecting a WebSocket client
// to /v1/ws and then sending POST /v1/notify produces broadcast
// messages for each state transition (at minimum "sending" and
// "delivered"). Satisfies notification-realtime REQ-024, REQ-025.
func TestWebSocketBroadcast(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)
	wsURL := fmt.Sprintf("ws://%s/v1/ws", addr)

	// Step 1: Connect WebSocket client.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.CloseNow()

	// Step 2: Send POST /v1/notify.
	email := "ws-e2e@company.com"
	body := fmt.Sprintf(`{"email": %q}`, email)
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var notifyResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&notifyResp)
	notificationID := notifyResp.ID
	if notificationID == "" {
		t.Fatal("notification ID is empty")
	}

	// Step 3: Read WebSocket messages until we see both "sending"
	// and "delivered", or timeout.
	type wsMsg struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}

	seenStates := make(map[string]bool)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			// Timeout reading -- check if we have enough.
			break
		}

		var msg wsMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Errorf("invalid JSON from WebSocket: %s", data)
			continue
		}

		// REQ-025: verify message contains id and state.
		if msg.ID == "" {
			t.Errorf("WebSocket message missing 'id': %s", data)
		}
		if msg.State == "" {
			t.Errorf("WebSocket message missing 'state': %s", data)
		}

		// Only track messages for our notification.
		if msg.ID == notificationID {
			t.Logf("received WS: id=%s state=%s", msg.ID, msg.State)
			seenStates[msg.State] = true
		}

		if seenStates["sending"] && seenStates["delivered"] {
			break
		}
	}

	// REQ-024: verify we saw at minimum "sending" and "delivered".
	if !seenStates["sending"] {
		t.Error("did not receive WebSocket broadcast for 'sending' state")
	}
	if !seenStates["delivered"] {
		t.Error("did not receive WebSocket broadcast for 'delivered' state")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}
```

**Step 2: Run the test**

Run: `cd tests/e2e && go test -v -run TestWebSocketBroadcast -timeout 60s`
Expected: PASS. Output shows received WS messages for "sending" and
"delivered" states.

**Step 3: Commit**

`test(e2e): add WebSocket broadcast E2E test (REQ-024, REQ-025)`

### Task 3: E2E test -- Pagination edge cases

**Files:**
- Create: `tests/e2e/pagination_test.go`

Satisfies notification-management REQ-028.

**Step 1: Write the test**

```go
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// paginationListResponse captures the list response shape for
// pagination tests.
type paginationListResponse struct {
	Notifications []json.RawMessage `json:"notifications"`
	Meta          struct {
		HasMore    bool `json:"has_more"`
		TotalCount int  `json:"total_count"`
		TotalPages int  `json:"total_pages"`
	} `json:"meta"`
}

// TestPaginationEmptyList verifies that an empty notification list
// returns the correct metadata. Satisfies REQ-028(c).
func TestPaginationEmptyList(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	resp, err := http.Get(base + "/v1/notifications")
	if err != nil {
		t.Fatalf("GET /v1/notifications: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body paginationListResponse
	json.NewDecoder(resp.Body).Decode(&body)

	if body.Meta.TotalCount != 0 {
		t.Errorf("total_count = %d, want 0", body.Meta.TotalCount)
	}
	if body.Meta.TotalPages != 0 {
		t.Errorf("total_pages = %d, want 0", body.Meta.TotalPages)
	}
	if body.Meta.HasMore {
		t.Error("has_more = true, want false")
	}
	if len(body.Notifications) != 0 {
		t.Errorf("notifications length = %d, want 0", len(body.Notifications))
	}
}

// TestPaginationLimitClamping verifies that limit values above 100
// are silently clamped to 100. Satisfies REQ-028(a).
func TestPaginationLimitClamping(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// Create a single notification so the list is not empty.
	body := `{"email": "clamp-test@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Request with limit=200 (above max 100).
	resp, err = http.Get(base + "/v1/notifications?limit=200")
	if err != nil {
		t.Fatalf("GET /v1/notifications?limit=200: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var listResp paginationListResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	// With 1 notification and limit clamped to 100, we should get
	// at most 100 results (here just 1).
	if len(listResp.Notifications) > 100 {
		t.Errorf("notifications length = %d, want <= 100 (clamped)", len(listResp.Notifications))
	}
}

// TestPaginationTotalPages verifies total_pages is correctly
// calculated as ceil(total_count / limit). Satisfies REQ-028(b).
func TestPaginationTotalPages(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// Create 3 notifications.
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"email": "pages-test-%d@company.com"}`, i)
		resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /v1/notify: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("notify %d: expected 202, got %d", i, resp.StatusCode)
		}
	}

	// Request with limit=2 -- 3 items / 2 per page = 2 pages.
	resp, err := http.Get(base + "/v1/notifications?limit=2")
	if err != nil {
		t.Fatalf("GET /v1/notifications?limit=2: %v", err)
	}
	defer resp.Body.Close()

	var listResp paginationListResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	if listResp.Meta.TotalCount != 3 {
		t.Errorf("total_count = %d, want 3", listResp.Meta.TotalCount)
	}
	if listResp.Meta.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2 (ceil(3/2))", listResp.Meta.TotalPages)
	}
	if !listResp.Meta.HasMore {
		t.Error("has_more = false, want true")
	}
	if len(listResp.Notifications) != 2 {
		t.Errorf("notifications length = %d, want 2", len(listResp.Notifications))
	}
}
```

**Step 2: Run the tests**

Run: `cd tests/e2e && go test -v -run TestPagination -timeout 60s`
Expected: All three pagination tests pass.

**Step 3: Commit**

`test(e2e): add pagination edge-case tests (REQ-028)`

### Task 4: E2E test -- Health endpoint

**Files:**
- Create: `tests/e2e/health_test.go`

Satisfies notification-management REQ-026.

**Step 1: Write the test**

```go
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestHealthEndpoint verifies that GET /v1/health returns 200 with
// {"status": "healthy"} and Content-Type: application/json. This is
// a dedicated test, not a side-effect assertion in another test.
// Satisfies notification-management REQ-026.
func TestHealthEndpoint(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	resp, err := http.Get(fmt.Sprintf("http://%s/v1/health", addr))
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()

	// Verify HTTP 200.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify Content-Type header.
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// Verify response body.
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want healthy", body["status"])
	}
}
```

**Step 2: Run the test**

Run: `cd tests/e2e && go test -v -run TestHealthEndpoint -timeout 30s`
Expected: PASS.

**Step 3: Commit**

`test(e2e): add dedicated health endpoint test (REQ-026)`

### Task 5: E2E test -- Request ID in Mailpit email headers

**Files:**
- Create: `tests/e2e/requestid_test.go`

Satisfies notification-delivery REQ-031.

**Step 1: Write the test**

```go
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRequestIDInEmail verifies that the X-Request-ID from the HTTP
// response is propagated into the email headers in Mailpit. Satisfies
// notification-delivery REQ-031.
func TestRequestIDInEmail(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send POST /v1/notify and capture X-Request-ID.
	body := `{"email": "reqid-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	httpRequestID := resp.Header.Get("X-Request-ID")
	if httpRequestID == "" {
		t.Fatal("X-Request-ID header missing from HTTP response")
	}
	t.Logf("HTTP X-Request-ID: %s", httpRequestID)

	// Step 2: Wait for email delivery to Mailpit.
	var mailpitMessages struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
		Total int `json:"total"`
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		json.NewDecoder(r.Body).Decode(&mailpitMessages)
		r.Body.Close()
		if mailpitMessages.Total > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if mailpitMessages.Total == 0 {
		t.Fatal("no email received in Mailpit within timeout")
	}

	// Step 3: Retrieve the email from Mailpit and check headers.
	msgID := mailpitMessages.Messages[0].ID
	msgResp, err := http.Get(fmt.Sprintf("%s/api/v1/message/%s/headers", mailpitAPI, msgID))
	if err != nil {
		t.Fatalf("get mailpit headers: %v", err)
	}
	defer msgResp.Body.Close()

	// Mailpit returns headers as a map of string -> []string.
	var headers map[string][]string
	if err := json.NewDecoder(msgResp.Body).Decode(&headers); err != nil {
		t.Fatalf("decode headers: %v", err)
	}

	// Step 4: Verify X-Request-ID in email matches HTTP response.
	emailReqIDs, ok := headers["X-Request-Id"]
	if !ok {
		// Try case variations -- Mailpit may normalise header names.
		emailReqIDs, ok = headers["X-Request-ID"]
	}
	if !ok {
		t.Fatalf("X-Request-ID header not found in email headers: %v", headers)
	}

	found := false
	for _, v := range emailReqIDs {
		if v == httpRequestID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("email X-Request-ID = %v, want %q", emailReqIDs, httpRequestID)
	}
}
```

**Step 2: Run the test**

Run: `cd tests/e2e && go test -v -run TestRequestIDInEmail -timeout 60s`
Expected: PASS. Log output shows the matching request IDs.

**Step 3: Commit**

`test(e2e): add Request-ID email header propagation test (REQ-031)`

### Task 6: Integration test -- Retry lifecycle (pending -> ... -> delivered)

**Files:**
- Create: `internal/infra/sqlite/retry_integration_test.go`

Satisfies notification-state-machine REQ-026.

**Step 1: Write the test**

```go
package sqlite

import (
	"context"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// TestRetryLifecycleIntegration exercises the full retry lifecycle
// (pending -> sending -> not_sent -> sending -> delivered) using real
// SQLite storage. Satisfies notification-state-machine REQ-026.
func TestRetryLifecycleIntegration(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_retry-lifecycle-1",
		Email:      "retry-lifecycle@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Transition 1: pending -> sending.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Transition 2: sending -> not_sent (soft fail, retries remain).
	if err := sm.FireCtx(ctx, domain.TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}
	// Increment retry_count in DB (mimics worker behaviour).
	n.RetryCount = 1
	n.Status = domain.StatusNotSent
	if err := store.UpdateNotification(ctx, n); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	// Transition 3: not_sent -> sending (retry).
	// Need new SM with updated retry_count.
	sm = domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerRetry); err != nil {
		t.Fatalf("Fire(TriggerRetry): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Transition 4: sending -> delivered.
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}
	if err := store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusDelivered, domain.TriggerDelivered); err != nil {
		t.Fatalf("LogTransition: %v", err)
	}

	// Verify final state in database.
	got, err := store.GetNotificationByEmail(ctx, "retry-lifecycle@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log has exactly 4 entries.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("audit log entries = %d, want 4", count)
	}
}
```

**Step 2: Run the test**

Run: `go test -run TestRetryLifecycleIntegration ./internal/infra/sqlite/...`
Expected: PASS.

**Step 3: Commit**

`test(integration): add retry lifecycle test with real SQLite (REQ-026)`

### Task 7: Integration test -- Retry limit exhaustion path

**Files:**
- Modify: `internal/infra/sqlite/retry_integration_test.go`

Satisfies notification-state-machine REQ-027.

**Step 1: Write the test**

Append to `retry_integration_test.go`:

```go
// TestRetryLimitExhaustionIntegration exercises the retry-limit-
// exhaustion path (pending -> sending -> not_sent -> sending -> failed)
// using real SQLite storage. Satisfies notification-state-machine
// REQ-027.
func TestRetryLimitExhaustionIntegration(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_retry-exhaust-1",
		Email:      "retry-exhaust@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 1, // 1 retry allowed, so 2nd send attempt exhausts.
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Transition 1: pending -> sending.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusPending, domain.StatusSending, domain.TriggerSend)

	// Transition 2: sending -> not_sent (soft fail, 1 retry remains).
	if err := sm.FireCtx(ctx, domain.TriggerSoftFail); err != nil {
		t.Fatalf("Fire(TriggerSoftFail): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusNotSent, domain.TriggerSoftFail)
	n.RetryCount = 1
	n.Status = domain.StatusNotSent
	_ = store.UpdateNotification(ctx, n)

	// Transition 3: not_sent -> sending (retry).
	// retry_count (1) == retry_limit (1) -> guard will block soft_fail.
	sm = domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerRetry); err != nil {
		t.Fatalf("Fire(TriggerRetry): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusNotSent, domain.StatusSending, domain.TriggerRetry)

	// Verify soft_fail is now rejected by the guard.
	err = sm.FireCtx(ctx, domain.TriggerSoftFail)
	if err == nil {
		t.Fatal("expected TriggerSoftFail to be rejected by guard, got nil")
	}

	// Transition 4: sending -> failed (retries exhausted).
	if err := sm.FireCtx(ctx, domain.TriggerFailed); err != nil {
		t.Fatalf("Fire(TriggerFailed): %v", err)
	}
	_ = store.LogTransition(ctx, "notification", n.ID,
		domain.StatusSending, domain.StatusFailed, domain.TriggerFailed)

	// Verify final state.
	got, err := store.GetNotificationByEmail(ctx, "retry-exhaust@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed {
		t.Errorf("status = %v, want failed", got.Status)
	}

	// Verify audit log has exactly 4 entries.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("audit log entries = %d, want 4", count)
	}
}
```

**Step 2: Run the test**

Run: `go test -run TestRetryLimitExhaustionIntegration ./internal/infra/sqlite/...`
Expected: PASS.

**Step 3: Commit**

`test(integration): add retry-limit exhaustion test with real SQLite (REQ-027)`

### Task 8: Integration test -- Worker with real SQLite store

**Files:**
- Create: `internal/infra/sqlite/worker_integration_test.go`

Satisfies notification-state-machine REQ-028.

**Step 1: Write the test**

```go
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// mockSender is a controllable email sender for integration tests.
type mockSender struct {
	err error
}

func (m *mockSender) Send(_ context.Context, _ *domain.EmailMessage) error {
	return m.err
}

// TestWorkerIntegrationSuccessPath exercises the worker's Handle
// method with a real SQLite store (not spy/mock). Success path:
// pending -> sending -> delivered. Satisfies notification-state-machine
// REQ-028.
func TestWorkerIntegrationSuccessPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_worker-int-success",
		Email:      "worker-success@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{err: nil} // success
	worker := queue.NewEmailWorker(store, sender, nil)

	payload, _ := json.Marshal(queue.EmailJobPayload{
		NotificationID: n.ID,
		Email:          n.Email,
		RequestID:      "req_int-success",
	})

	if err := worker.Handle(ctx, payload); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	// Verify final state in real database.
	got, err := store.GetNotificationByEmail(ctx, n.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("status = %v, want delivered", got.Status)
	}

	// Verify audit log entries: pending->sending, sending->delivered.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}

// TestWorkerIntegrationSoftFailPath exercises the worker's Handle
// method with a real SQLite store. Soft-fail path: pending -> sending
// -> not_sent. Satisfies notification-state-machine REQ-028.
func TestWorkerIntegrationSoftFailPath(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_worker-int-softfail",
		Email:      "worker-softfail@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{err: errors.New("smtp timeout")} // transient fail
	worker := queue.NewEmailWorker(store, sender, nil)

	payload, _ := json.Marshal(queue.EmailJobPayload{
		NotificationID: n.ID,
		Email:          n.Email,
		RequestID:      "req_int-softfail",
	})

	// Should return error (goqite retries via visibility timeout).
	err = worker.Handle(ctx, payload)
	if err == nil {
		t.Fatal("expected error on soft fail, got nil")
	}

	// Verify final state in real database.
	got, err := store.GetNotificationByEmail(ctx, n.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusNotSent {
		t.Errorf("status = %v, want not_sent", got.Status)
	}
	if got.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", got.RetryCount)
	}

	// Verify audit log entries: pending->sending, sending->not_sent.
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = ?", n.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("audit log entries = %d, want 2", count)
	}
}
```

**Step 2: Run the tests**

Run: `go test -run TestWorkerIntegration ./internal/infra/sqlite/...`
Expected: PASS for both success and soft-fail paths.

**Step 3: Commit**

`test(integration): add worker integration tests with real SQLite (REQ-028)`

### Task 9: Integration test -- Queue enqueue-dequeue cycle

**Files:**
- Modify: `internal/infra/queue/queue_test.go`

Satisfies notification-delivery REQ-032.

**Step 1: Write the test**

Append to `queue_test.go`:

```go
// TestQueueEnqueueDequeueIntegration exercises the full enqueue ->
// dequeue -> verify payload cycle using real goqite with a real SQLite
// database. Satisfies notification-delivery REQ-032.
func TestQueueEnqueueDequeueIntegration(t *testing.T) {
	db := setupTestDB(t)
	nq, err := NewNotificationQueue(db, FlavorSQLite)
	if err != nil {
		t.Fatalf("NewNotificationQueue() error: %v", err)
	}

	// Step 1: Create a payload and enqueue it.
	originalPayload := EmailJobPayload{
		NotificationID: "ntf_queue-int-1",
		Email:          "queue-int@test.com",
		RequestID:      "req_queue-int-1",
	}
	data, _ := json.Marshal(originalPayload)

	if err := nq.Enqueue(context.Background(), data); err != nil {
		t.Fatalf("Enqueue() error: %v", err)
	}

	// Step 2: Dequeue using goqite's Receive() directly.
	msg, err := nq.Queue().Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if msg == nil {
		t.Fatal("Receive() returned nil message")
	}

	// Step 3: The received body is wrapped in a jobs.Create envelope.
	// The envelope is JSON: {"job": "<name>", "body": <raw>}.
	var envelope struct {
		Job  string          `json:"job"`
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(msg.Body, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if envelope.Job != "send_notification" {
		t.Errorf("envelope job = %q, want send_notification", envelope.Job)
	}

	// Step 4: Verify the inner payload matches what was enqueued.
	var received EmailJobPayload
	if err := json.Unmarshal(envelope.Body, &received); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if received.NotificationID != originalPayload.NotificationID {
		t.Errorf("NotificationID = %q, want %q",
			received.NotificationID, originalPayload.NotificationID)
	}
	if received.Email != originalPayload.Email {
		t.Errorf("Email = %q, want %q", received.Email, originalPayload.Email)
	}
	if received.RequestID != originalPayload.RequestID {
		t.Errorf("RequestID = %q, want %q",
			received.RequestID, originalPayload.RequestID)
	}
}
```

**Step 2: Run the test**

Run: `go test -run TestQueueEnqueueDequeueIntegration ./internal/infra/queue/...`
Expected: PASS.

**Step 3: Commit**

`test(integration): add queue enqueue-dequeue cycle test (REQ-032)`

### Task 10: Integration test -- Retry guard rejection with real database

**Files:**
- Create: `internal/infra/sqlite/resetguard_integration_test.go`

Satisfies notification-management REQ-029.

**Step 1: Write the test**

```go
package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// TestResetGuardRejectRetryInProgress verifies that the reset guard
// returns ErrRetriesRemaining when a notification is in not_sent state
// with retry_count < retry_limit, using a real SQLite database.
// Satisfies notification-management REQ-029.
func TestResetGuardRejectRetryInProgress(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create notification in not_sent with retries remaining.
	n := &domain.Notification{
		ID:         "ntf_guard-reject-1",
		Email:      "guard-reject@test.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Read it back from the database to confirm state.
	got, err := store.GetNotificationByEmail(ctx, "guard-reject@test.com")
	if err != nil {
		t.Fatal(err)
	}

	// Invoke the guard.
	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err == nil {
		t.Fatal("expected ErrRetriesRemaining, got nil")
	}
	if !errors.Is(err, domain.ErrRetriesRemaining) {
		t.Errorf("error = %v, want ErrRetriesRemaining", err)
	}
}

// TestResetGuardAllowRetriesExhausted verifies that the reset guard
// allows reset when retry_count >= retry_limit, using a real SQLite
// database. Satisfies notification-management REQ-029.
func TestResetGuardAllowRetriesExhausted(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create notification in not_sent with retries exhausted.
	n := &domain.Notification{
		ID:         "ntf_guard-allow-1",
		Email:      "guard-allow@test.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-allow@test.com")
	if err != nil {
		t.Fatal(err)
	}

	// Invoke the guard.
	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed), got %v", err)
	}
}

// TestResetGuardAllowFailedState verifies that failed notifications
// can always be reset regardless of retry_count.
func TestResetGuardAllowFailedState(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	n := &domain.Notification{
		ID:         "ntf_guard-failed-1",
		Email:      "guard-failed@test.com",
		Status:     domain.StatusFailed,
		RetryCount: 1,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}

	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed for failed), got %v", err)
	}
}

// TestResetGuardAllowDeliveredState verifies that delivered
// notifications can always be reset regardless of retry_count.
func TestResetGuardAllowDeliveredState(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	n := &domain.Notification{
		ID:         "ntf_guard-delivered-1",
		Email:      "guard-delivered@test.com",
		Status:     domain.StatusDelivered,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNotificationByEmail(ctx, "guard-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}

	err = domain.CheckResetAllowed(got.Status, got.RetryCount, got.RetryLimit)
	if err != nil {
		t.Errorf("expected nil (reset allowed for delivered), got %v", err)
	}
}
```

**Step 2: Run the tests**

Run: `go test -run TestResetGuard ./internal/infra/sqlite/...`
Expected: All four guard tests pass.

**Step 3: Commit**

`test(integration): add reset guard tests with real SQLite (REQ-029)`

### Task 11: Run full test suite

**Depends on:** Tasks 1-10

**Files:** (none new)

**Step 1: Run unit tests**

Run: `mise run test:unit`
Expected: All tests pass, including the new integration tests.

**Step 2: Run lint**

Run: `mise run lint`
Expected: No warnings or errors.

**Step 3: Run E2E tests**

Run: `mise run test:e2e`
Expected: All tests pass, including the four new E2E tests.

## Verification Checklist

- [ ] `mise run build:go` succeeds with no warnings
- [ ] `mise run test:unit` -- all tests pass (existing + 9 new integration tests)
- [ ] `mise run lint:go` -- no warnings
- [ ] `mise run test:e2e` -- all tests pass (existing + 4 new E2E tests)
- [ ] `TestWebSocketBroadcast` receives "sending" and "delivered" WS messages (REQ-024, REQ-025)
- [ ] `TestPaginationEmptyList` verifies 0/0/false/empty (REQ-028c)
- [ ] `TestPaginationLimitClamping` verifies at-most-100 with limit=200 (REQ-028a)
- [ ] `TestPaginationTotalPages` verifies ceil(3/2)=2 (REQ-028b)
- [ ] `TestHealthEndpoint` verifies 200 + healthy + content-type (REQ-026)
- [ ] `TestRequestIDInEmail` verifies X-Request-ID matches HTTP -> Mailpit (REQ-031)
- [ ] `TestRetryLifecycleIntegration` verifies 4-transition retry path with real SQLite (REQ-026)
- [ ] `TestRetryLimitExhaustionIntegration` verifies guard rejection + failed (REQ-027)
- [ ] `TestWorkerIntegrationSuccessPath` verifies worker with real store (REQ-028)
- [ ] `TestWorkerIntegrationSoftFailPath` verifies worker soft-fail with real store (REQ-028)
- [ ] `TestQueueEnqueueDequeueIntegration` verifies full enqueue-dequeue cycle (REQ-032)
- [ ] `TestResetGuardRejectRetryInProgress` verifies guard rejects not_sent+retries (REQ-029)
- [ ] `TestResetGuardAllowRetriesExhausted` verifies guard allows not_sent+exhausted (REQ-029)
- [ ] `TestResetGuardAllowFailedState` verifies guard allows failed state
- [ ] `TestResetGuardAllowDeliveredState` verifies guard allows delivered state
- [ ] No production code changed -- test-only additions
