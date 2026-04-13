---
type: plan
step: "21"
title: "Fix reset handler status overwrite bug"
status: pending
assessment_status: needed
provenance:
  source: issue
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-4-state-machine
  - step-5-reset-list-postgres
---

# Step 21: Fix reset handler status overwrite bug

## Overview

Both reset handlers (HTTP and MCP) have a bug where the state machine
mutator writes `pending` to the DB, but the subsequent
`UpdateNotification(n)` call overwrites it with the stale `n.Status`
(e.g. `failed`) from the original `GetNotificationByEmail` lookup. The
in-memory struct `n` is never updated to reflect the state machine
transition before it is written back.

**Root cause:** `sm.FireCtx` drives the mutator which writes the new
status to the DB, but does not update the caller's local `n.Status`.
When the handler later calls `store.UpdateNotification(ctx, n)` to
clear `RetryCount` and `UpdatedAt`, it writes `n.Status` (still the
old value) back to the DB.

**Fix:** After `sm.FireCtx` succeeds, set `n.Status = domain.StatusPending`
before calling `UpdateNotification`. This is a one-line fix in each
handler.

**Why the existing tests don't catch it:** The stub store's
`GetNotificationByEmail` returns the same pointer stored in the map.
The mutator writes to that same pointer, so `n.Status` is already
updated by the time `UpdateNotification` runs. A real SQLite/Postgres
store returns a copy from `GetNotificationByEmail` (scanned from a
query result), so the mutator updates the DB row but not the caller's
local struct.

**Test strategy:** This plan fixes both handlers, hardens the unit
test stubs to return copies (preventing this class of bug from hiding
again), adds integration tests against the real SQLite store, and adds
E2E tests covering the full reset-to-redelivery cycle.

## Prerequisites

- Familiarity with `internal/domain/statemachine.go` (state machine
  configuration and mutator pattern).
- Mailpit running for E2E tests (`mise run services:up` or equivalent).

## Task breakdown

### Task 1: Fix the HTTP reset handler and add a copy-store regression test

**Files:**
- Modify: `internal/infra/httpapi/reset.go:85-90`
- Modify: `internal/infra/httpapi/reset_test.go`

**Step 1: Add a copy-returning stub and regression test**

Add a test that uses a store stub where `GetNotificationByEmail`
returns a *copy* of the notification (not a pointer into the map), so
the mutator and the caller's `n` are decoupled. This simulates real
SQLite/Postgres behavior.

In `internal/infra/httpapi/reset_test.go`, add after the existing
tests:

```go
// resetCopyStore returns a copy from GetNotificationByEmail so that the
// state-machine mutator and the caller's local struct are decoupled,
// matching real SQLite/Postgres store behavior.
type resetCopyStore struct {
	resetStubStore
}

func newResetCopyStore() *resetCopyStore {
	return &resetCopyStore{
		resetStubStore: resetStubStore{
			stubNotificationStore: stubNotificationStore{
				notifications: make(map[string]*domain.Notification),
			},
		},
	}
}

func (s *resetCopyStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	// Return a copy, not a pointer into the map. This matches real
	// store behavior where the struct is scanned from a query result.
	cp := *n
	return &cp, nil
}

func TestHandleResetStatusNotOverwritten(t *testing.T) {
	store := newResetCopyStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_overwrite-1",
		Email:      "user@company.com",
		Status:     domain.StatusFailed,
		RetryCount: 3,
		RetryLimit: domain.DefaultRetryLimit,
	}

	enqueuer := &stubEnqueuer{}
	handler := HandleReset(store, enqueuer)

	body := `{"email": "user@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// The critical assertion: the notification in the store must be
	// pending, not failed. Before the fix, UpdateNotification would
	// overwrite the mutator's pending with the stale failed status.
	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v (status was overwritten by stale struct)", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `mise run test -- -run TestHandleResetStatusNotOverwritten ./internal/infra/httpapi/...`
Expected: FAIL -- `status = failed, want pending (status was overwritten by stale struct)`

**Step 3: Apply the one-line fix**

In `internal/infra/httpapi/reset.go`, after the `LogTransition` block
(after line 83) and before the retry-count clearing block, add:

```go
	// Sync the in-memory struct with the status the mutator wrote to
	// the DB, so UpdateNotification does not overwrite it.
	n.Status = domain.StatusPending
```

The modified section (lines 84-92) should read:

```go
	// Sync the in-memory struct with the status the mutator wrote to
	// the DB, so UpdateNotification does not overwrite it.
	n.Status = domain.StatusPending

	// Clear retry count and timestamps (except created_at).
	// REQ-004: clear retry_count. REQ-005: clear delivery results
	// (status already reset by state machine; retry_count is the
	// remaining delivery result field). REQ-006: reset timestamps.
	n.RetryCount = 0
	n.UpdatedAt = time.Time{}
```

**Step 4: Run the test to verify it passes**

Run: `mise run test -- -run TestHandleResetStatusNotOverwritten ./internal/infra/httpapi/...`
Expected: PASS

**Step 5: Run all reset tests to verify no regressions**

Run: `mise run test -- -run TestHandleReset ./internal/infra/httpapi/...`
Expected: PASS (all reset tests)

### Task 2: Fix the MCP reset handler and add a copy-store regression test

**Files:**
- Modify: `internal/infra/mcp/tools.go:105-107`
- Modify: `internal/infra/mcp/tools_test.go`

**Step 1: Add a copy-returning stub and regression test**

In `internal/infra/mcp/tools_test.go`, add a copy-returning store and
test:

```go
// resetCopyStore returns a copy from GetNotificationByEmail to decouple
// the state-machine mutator from the caller's struct.
type resetCopyStore struct {
	stubNotificationStore
}

func newResetCopyStore() *resetCopyStore {
	return &resetCopyStore{
		stubNotificationStore: stubNotificationStore{
			notifications: make(map[string]*domain.Notification),
		},
	}
}

func (s *resetCopyStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *n
	return &cp, nil
}

func TestResetNotificationStatusNotOverwritten(t *testing.T) {
	store := newResetCopyStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_overwrite-mcp",
		Email:      "user@company.com",
		Status:     domain.StatusFailed,
		RetryCount: 3,
		RetryLimit: domain.DefaultRetryLimit,
	}

	enqueuer := &stubEnqueuer{}
	handler := HandleResetNotification(store, enqueuer)

	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "user@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v (status was overwritten by stale struct)", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `mise run test -- -run TestResetNotificationStatusNotOverwritten ./internal/infra/mcp/...`
Expected: FAIL -- `status = failed, want pending`

**Step 3: Apply the one-line fix**

In `internal/infra/mcp/tools.go`, after the `LogTransition` call
(line 104) and before the retry-count clearing block, add:

```go
		// Sync the in-memory struct with the status the mutator wrote to
		// the DB, so UpdateNotification does not overwrite it.
		n.Status = domain.StatusPending
```

The modified section (lines 102-111) should read:

```go
		//nolint:errcheck // Log failure is non-fatal; reset already succeeded.
		_ = store.LogTransition(ctx, "notification", n.ID,
			prevStatus, domain.StatusPending, domain.TriggerReset)

		// Sync the in-memory struct with the status the mutator wrote to
		// the DB, so UpdateNotification does not overwrite it.
		n.Status = domain.StatusPending

		n.RetryCount = 0
		n.UpdatedAt = time.Time{}
```

**Step 4: Run the test to verify it passes**

Run: `mise run test -- -run TestResetNotificationStatusNotOverwritten ./internal/infra/mcp/...`
Expected: PASS

**Step 5: Run all MCP tests to verify no regressions**

Run: `mise run test -- ./internal/infra/mcp/...`
Expected: PASS

**Step 6: Commit**

`fix(reset): sync n.Status after state machine fire to prevent overwrite`

### Task 3: Integration tests -- reset state transitions through real SQLite store

**Files:**
- Modify: `internal/infra/sqlite/statemachine_integration_test.go`

These tests exercise the full reset flow through the real SQLite store,
verifying that `GetNotificationByEmail` returns a copy and the handler
pattern (fire + update) produces the correct final state.

**Step 1: Add integration test for reset from failed**

Append to `internal/infra/sqlite/statemachine_integration_test.go`:

```go
func TestStateMachineIntegrationResetFromFailed(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-reset-failed",
		Email:      "reset-failed@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Drive to failed: pending -> sending -> failed.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerFailed); err != nil {
		t.Fatalf("Fire(TriggerFailed): %v", err)
	}

	// Simulate the handler pattern: get a fresh copy, fire reset,
	// then sync the local struct and update.
	got, err := store.GetNotificationByEmail(ctx, "reset-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed {
		t.Fatalf("pre-reset status = %v, want failed", got.Status)
	}

	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatalf("Fire(TriggerReset): %v", err)
	}

	// This is the bug fix line -- without it, got.Status is still "failed".
	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	// Verify final state in database via a fresh read.
	final, err := store.GetNotificationByEmail(ctx, "reset-failed@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", final.Status)
	}
	if final.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", final.RetryCount)
	}
}
```

**Step 2: Add integration test for reset from delivered**

```go
func TestStateMachineIntegrationResetFromDelivered(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-reset-delivered",
		Email:      "reset-delivered@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Drive to delivered: pending -> sending -> delivered.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("Fire(TriggerSend): %v", err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("Fire(TriggerDelivered): %v", err)
	}

	// Simulate the handler pattern with fresh copy.
	got, err := store.GetNotificationByEmail(ctx, "reset-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusDelivered {
		t.Fatalf("pre-reset status = %v, want delivered", got.Status)
	}

	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatalf("Fire(TriggerReset): %v", err)
	}

	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatalf("UpdateNotification: %v", err)
	}

	final, err := store.GetNotificationByEmail(ctx, "reset-delivered@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", final.Status)
	}
	if final.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", final.RetryCount)
	}
}
```

**Step 3: Add integration test for re-delivery after reset**

This test verifies the full cycle: reset to pending, then the worker
path (pending -> sending -> delivered) succeeds on the reset
notification.

```go
func TestStateMachineIntegrationRedeliveryAfterReset(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_sm-redeliver",
		Email:      "redeliver@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: 3,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	// First delivery: pending -> sending -> delivered.
	sm := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(n.ID),
		store.NotificationStateMutator(n.ID),
		n.RetryLimit, n.RetryCount,
	)
	if err := sm.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatal(err)
	}
	if err := sm.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatal(err)
	}

	// Reset: delivered -> pending (simulating handler pattern).
	got, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	resetSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(got.ID),
		store.NotificationStateMutator(got.ID),
		got.RetryLimit, got.RetryCount,
	)
	if err := resetSM.FireCtx(ctx, domain.TriggerReset); err != nil {
		t.Fatal(err)
	}
	got.Status = domain.StatusPending
	got.RetryCount = 0
	if err := store.UpdateNotification(ctx, got); err != nil {
		t.Fatal(err)
	}

	// Re-delivery: pending -> sending -> delivered (simulating worker).
	reread, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	workerSM := domain.ConfigureStateMachine(
		store.NotificationStateAccessor(reread.ID),
		store.NotificationStateMutator(reread.ID),
		reread.RetryLimit, reread.RetryCount,
	)
	if err := workerSM.FireCtx(ctx, domain.TriggerSend); err != nil {
		t.Fatalf("re-delivery TriggerSend: %v", err)
	}

	// Verify intermediate state is sending.
	mid, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if mid.Status != domain.StatusSending {
		t.Errorf("mid-delivery status = %v, want sending", mid.Status)
	}

	if err := workerSM.FireCtx(ctx, domain.TriggerDelivered); err != nil {
		t.Fatalf("re-delivery TriggerDelivered: %v", err)
	}

	// Verify final state.
	final, err := store.GetNotificationByEmail(ctx, "redeliver@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.StatusDelivered {
		t.Errorf("final status = %v, want delivered", final.Status)
	}
}
```

**Step 4: Run the integration tests**

Run: `mise run test -- -run TestStateMachineIntegrationReset ./internal/infra/sqlite/...`
Expected: PASS (all three new tests)

Run: `mise run test -- ./internal/infra/sqlite/...`
Expected: PASS (all existing + new tests)

**Step 5: Commit**

`test(reset): add integration tests for reset state transitions through SQLite`

### Task 4: E2E test -- reset and redeliver

**Files:**
- Create: `tests/e2e/reset_test.go`

This E2E test sends a notification, waits for delivery, resets it via
the REST API, and waits for the re-delivery to arrive in Mailpit.

**Step 1: Write the E2E test**

Create `tests/e2e/reset_test.go`:

```go
package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// pollNotificationState polls GET /v1/notifications until the
// notification with the given email reaches the target state, or the
// timeout expires. Returns the final observed state.
func pollNotificationState(t *testing.T, base, email, targetState string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastState string
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v1/notifications?limit=100")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var listResp struct {
			Notifications []struct {
				Email string `json:"email"`
				State string `json:"state"`
			} `json:"notifications"`
		}
		json.NewDecoder(resp.Body).Decode(&listResp)
		resp.Body.Close()

		for _, n := range listResp.Notifications {
			if n.Email == email {
				lastState = n.State
				if n.State == targetState {
					return n.State
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastState
}

// pollMailpitCount polls Mailpit until at least targetCount messages
// exist, or the timeout expires. Returns the final count.
func pollMailpitCount(t *testing.T, mailpitAPI string, targetCount int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var count int
	for time.Now().Before(deadline) {
		resp, err := http.Get(mailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var msgs struct {
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&msgs)
		resp.Body.Close()
		count = msgs.Total
		if count >= targetCount {
			return count
		}
		time.Sleep(500 * time.Millisecond)
	}
	return count
}

// TestResetAndRedeliver verifies the full cycle: notify -> deliver ->
// reset -> re-deliver. This is the primary E2E test for the reset bug
// fix.
func TestResetAndRedeliver(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send notification.
	body := `{"email": "reset-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Step 2: Wait for delivery.
	state := pollNotificationState(t, base, "reset-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	mailCount := pollMailpitCount(t, mailpitAPI, 1, 5*time.Second)
	if mailCount < 1 {
		t.Fatal("no email received in Mailpit before reset")
	}

	// Step 3: Reset.
	resetBody := `{"email": "reset-e2e@company.com"}`
	resp, err = http.Post(base+"/v1/notify/reset", "application/json", strings.NewReader(resetBody))
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d: %s", resp.StatusCode, respBody)
	}

	// Step 4: Verify state went back to pending (briefly).
	// Poll for delivered again -- the re-delivery should happen.
	state = pollNotificationState(t, base, "reset-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("after reset: expected re-delivery to delivered, got %q", state)
	}

	// Step 5: Verify a second email arrived in Mailpit.
	mailCount = pollMailpitCount(t, mailpitAPI, 2, 10*time.Second)
	if mailCount < 2 {
		t.Errorf("expected at least 2 emails in Mailpit after re-delivery, got %d", mailCount)
	}
}

// TestResetFailedExample verifies that resetting a notification that
// failed (e.g. @example.com) results in it failing again after
// re-delivery, not getting stuck in a stale state.
func TestResetFailedExample(t *testing.T) {
	smtpHost, smtpPort, _ := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send notification to example.com (auto-fails).
	body := `{"email": "reset-e2e@example.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Step 2: Wait for it to reach a terminal failure state.
	// example.com triggers immediate failure, but retries may occur.
	// Wait for either "failed" or "not_sent" with exhausted retries.
	var finalState string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		state := pollNotificationState(t, base, "reset-e2e@example.com", "failed", 2*time.Second)
		if state == "failed" {
			finalState = state
			break
		}
		// Also check not_sent -- the worker may leave it there if
		// retries are exhausted.
		state = pollNotificationState(t, base, "reset-e2e@example.com", "not_sent", 2*time.Second)
		if state == "not_sent" {
			finalState = state
			break
		}
		time.Sleep(1 * time.Second)
	}
	if finalState != "failed" && finalState != "not_sent" {
		t.Fatalf("expected failed or not_sent, got %q", finalState)
	}

	// Step 3: Reset the notification.
	resetBody := `{"email": "reset-e2e@example.com"}`
	resp, err = http.Post(base+"/v1/notify/reset", "application/json", strings.NewReader(resetBody))
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d", resp.StatusCode)
	}

	// Step 4: Wait for it to fail again after re-delivery attempt.
	finalState = ""
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		state := pollNotificationState(t, base, "reset-e2e@example.com", "failed", 2*time.Second)
		if state == "failed" {
			finalState = state
			break
		}
		state = pollNotificationState(t, base, "reset-e2e@example.com", "not_sent", 2*time.Second)
		if state == "not_sent" {
			finalState = state
			break
		}
		time.Sleep(1 * time.Second)
	}
	if finalState != "failed" && finalState != "not_sent" {
		t.Fatalf("after reset: expected failure again, got %q", finalState)
	}
}

// TestResetRetryCount verifies that retry_count is reset to 0 after a
// reset, observable through the list endpoint.
func TestResetRetryCount(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)

	// Step 1: Send and wait for delivery.
	body := `{"email": "retry-count-e2e@company.com"}`
	resp, err := http.Post(base+"/v1/notify", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()

	state := pollNotificationState(t, base, "retry-count-e2e@company.com", "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	// Step 2: Reset.
	resetBody := `{"email": "retry-count-e2e@company.com"}`
	resp, err = http.Post(base+"/v1/notify/reset", "application/json", strings.NewReader(resetBody))
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d", resp.StatusCode)
	}

	// Step 3: Immediately check that retry_count is 0.
	// The reset handler clears retry_count synchronously before
	// returning 204, so it should be observable right away.
	resp, err = http.Get(base + "/v1/notifications?limit=100")
	if err != nil {
		t.Fatalf("GET /v1/notifications: %v", err)
	}
	var listResp struct {
		Notifications []struct {
			Email      string `json:"email"`
			State      string `json:"state"`
			RetryCount int    `json:"retry_count"`
		} `json:"notifications"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	found := false
	for _, n := range listResp.Notifications {
		if n.Email == "retry-count-e2e@company.com" {
			found = true
			if n.RetryCount != 0 {
				t.Errorf("retry_count = %d, want 0 after reset", n.RetryCount)
			}
			// State should be pending (or sending if worker is fast).
			if n.State != "pending" && n.State != "sending" && n.State != "delivered" {
				t.Errorf("state = %q, want pending/sending/delivered", n.State)
			}
		}
	}
	if !found {
		t.Fatal("notification not found in list response")
	}
}
```

**Step 2: Run the E2E tests**

Run: `mise run test -- -run "TestReset" -timeout 120s ./tests/e2e/...`
Expected: PASS (all three E2E tests)

**Step 3: Commit**

`test(e2e): add reset-and-redeliver E2E tests`

### Task 5: Harden existing unit test stubs to return copies

**Files:**
- Modify: `internal/infra/httpapi/reset_test.go`
- Modify: `internal/infra/mcp/tools_test.go`

The existing stubs return the same pointer from the map, which masks
pointer-aliasing bugs like the one fixed here. Update the base stubs
so `GetNotificationByEmail` returns a copy by default. This prevents
this class of bug from hiding in future tests.

**Step 1: Update the httpapi resetStubStore**

In `internal/infra/httpapi/reset_test.go`, modify the existing
`stubNotificationStore.GetNotificationByEmail` (inherited from
`notify_test.go` or local) -- if `resetStubStore` inherits it,
override it to return a copy. Since `resetCopyStore` was added in
Task 1, the simplest approach is to make `resetStubStore` itself
return copies. Replace the `resetStubStore` definition and remove the
separate `resetCopyStore`, since the base store now returns copies:

In `internal/infra/httpapi/reset_test.go`, update `resetStubStore` to
override `GetNotificationByEmail`:

```go
func (s *resetStubStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *n
	return &cp, nil
}
```

Then remove the `resetCopyStore` type and `newResetCopyStore` function
added in Task 1. Update `TestHandleResetStatusNotOverwritten` to use
`newResetStubStore()` instead of `newResetCopyStore()`.

**Step 2: Update the MCP stubNotificationStore**

In `internal/infra/mcp/tools_test.go`, update the existing
`stubNotificationStore.GetNotificationByEmail` to return a copy:

```go
func (s *stubNotificationStore) GetNotificationByEmail(_ context.Context, email string) (*domain.Notification, error) {
	n, ok := s.notifications[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *n
	return &cp, nil
}
```

Then remove the `resetCopyStore` type and `newResetCopyStore` function
added in Task 2. Update `TestResetNotificationStatusNotOverwritten`
to use `newStubStore()` instead of `newResetCopyStore()`.

**Step 3: Run all unit tests to verify nothing broke**

Run: `mise run test -- ./internal/infra/httpapi/...`
Expected: PASS

Run: `mise run test -- ./internal/infra/mcp/...`
Expected: PASS

**Step 4: Commit**

`refactor(test): make stub stores return copies to match real DB behavior`

## Verification checklist

- [ ] `TestHandleResetStatusNotOverwritten` passes (HTTP handler, copy store)
- [ ] `TestResetNotificationStatusNotOverwritten` passes (MCP handler, copy store)
- [ ] All existing reset unit tests still pass
- [ ] `TestStateMachineIntegrationResetFromFailed` passes (SQLite)
- [ ] `TestStateMachineIntegrationResetFromDelivered` passes (SQLite)
- [ ] `TestStateMachineIntegrationRedeliveryAfterReset` passes (SQLite)
- [ ] `TestResetAndRedeliver` passes (E2E: full reset-redeliver cycle)
- [ ] `TestResetFailedExample` passes (E2E: reset of failed notification)
- [ ] `TestResetRetryCount` passes (E2E: retry_count zeroed after reset)
- [ ] All existing unit/integration tests still pass: `mise run test`
- [ ] No data races: binary built with `-race` flag in E2E harness
