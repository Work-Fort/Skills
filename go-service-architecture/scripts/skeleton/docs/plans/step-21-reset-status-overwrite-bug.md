---
type: plan
step: "21"
title: "Fix reset handler status overwrite race"
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

# Step 21: Fix reset handler status overwrite race

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

**Why the existing tests don't catch it:** The stub mutator writes
directly to the same `*domain.Notification` pointer stored in the
in-memory map, so `n.Status` is already updated by the time
`UpdateNotification` runs. A real Postgres store returns a copy from
`GetNotificationByEmail`, so the mutator updates the DB row but not
the caller's local struct.

## Prerequisites

- Familiarity with `internal/domain/statemachine.go` (state machine
  configuration and mutator pattern).

## Task breakdown

### Task 1: Fix the HTTP reset handler and add a regression test

**Files:**
- Modify: `internal/infra/httpapi/reset.go:88`
- Modify: `internal/infra/httpapi/reset_test.go`

**Step 1: Add a regression test with a decoupled stub**

Add a test that uses a store stub where `GetNotificationByEmail`
returns a *copy* of the notification (not a pointer into the map), so
the mutator and the caller's `n` are decoupled. This simulates real
Postgres behavior.

In `internal/infra/httpapi/reset_test.go`, add after the existing
tests:

```go
// resetCopyStore returns a copy from GetNotificationByEmail so that the
// state-machine mutator and the caller's local struct are decoupled,
// matching real Postgres store behavior.
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
	// Return a copy, not a pointer into the map. This matches Postgres
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
}
```

**Step 2: Run the test to verify it fails**

Run: `mise run test -- -run TestHandleResetStatusNotOverwritten ./internal/infra/httpapi/...`
Expected: FAIL — `status = failed, want pending (status was overwritten by stale struct)`

**Step 3: Apply the one-line fix**

In `internal/infra/httpapi/reset.go`, after the `sm.FireCtx` block
(after line 76) and before the retry-count clearing block, add:

```go
	// Sync the in-memory struct with the status the mutator wrote to
	// the DB, so UpdateNotification does not overwrite it.
	n.Status = domain.StatusPending
```

The modified section (lines 86-92) should read:

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

### Task 2: Fix the MCP reset handler and add a regression test

**Files:**
- Modify: `internal/infra/mcp/tools.go:106`
- Modify: `internal/infra/mcp/tools_test.go`

**Step 1: Add a regression test with a decoupled stub**

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
}
```

**Step 2: Run the test to verify it fails**

Run: `mise run test -- -run TestResetNotificationStatusNotOverwritten ./internal/infra/mcp/...`
Expected: FAIL — `status = failed, want pending`

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

## Verification checklist

- [ ] `TestHandleResetStatusNotOverwritten` passes (HTTP handler)
- [ ] `TestResetNotificationStatusNotOverwritten` passes (MCP handler)
- [ ] All existing reset tests still pass
- [ ] Full test suite passes: `mise run test`
