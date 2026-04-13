---
type: plan
step: "20"
title: "Reset Enqueue and Button Min-Width"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "20"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
  - step-19-resend-ux-reset-guard
---

# Step 20: Reset Enqueue and Button Min-Width

## Overview

Two fixes in one step:

1. **Reset must enqueue a delivery job (REQ-007).** The reset endpoint
   transitions the notification back to `pending` and clears fields, but
   never enqueues a background job. The notification sits in `pending`
   forever with no worker picking it up. Both the HTTP handler and the
   MCP tool handler need the same fix.

2. **Button min-width too narrow (REQ-062).** The Resend button uses
   `min-w-[7rem]` (112px) but "Resending..." renders at 115.3px,
   causing a layout shift. Bump to `min-w-[7.5rem]` (120px).

## Prerequisites

- Step 19 (resend UX / reset guard) is complete.
- All tests pass on the current branch.

## Tasks

### Task 1: Add enqueuer to HTTP HandleReset

**Files:**
- Modify: `internal/infra/httpapi/reset.go:23` (signature)
- Modify: `internal/infra/httpapi/reset.go:91-101` (add enqueue after update)
- Modify: `internal/infra/httpapi/reset_test.go` (update calls, add assertion)
- Modify: `cmd/daemon/daemon.go:216` (pass enqueuer)

**Step 1: Write the failing test — assert enqueue on reset**

In `internal/infra/httpapi/reset_test.go`, update `TestHandleResetSuccess`
to pass a `stubEnqueuer` and assert a job was enqueued. The existing
`stubEnqueuer` from `notify_test.go` is in the same package and reusable.

Change the handler construction and add the enqueue assertion:

```go
func TestHandleResetSuccess(t *testing.T) {
	store := newResetStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_reset-1",
		Email:      "user@company.com",
		Status:     domain.StatusDelivered,
		RetryCount: 2,
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

	// 204 must have an empty body.
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}

	// Verify the notification was reset.
	n := store.notifications["user@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}

	// Verify a transition was logged.
	if len(store.transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(store.transitions))
	}
	if store.transitions[0] != "delivered->pending" {
		t.Errorf("transition = %q, want %q", store.transitions[0], "delivered->pending")
	}

	// REQ-007: Verify a delivery job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(enqueuer.jobs))
	}
}
```

Also update every other test that calls `HandleReset(store)` to pass a
nil or stub enqueuer. For tests that do not exercise the enqueue path
(not-found, invalid JSON, retries remaining, oversized body), pass
`&stubEnqueuer{}`:

```go
// In TestHandleResetNotFound, TestHandleResetInvalidJSON,
// TestHandleResetRetriesRemaining, TestHandleResetOversizedBody,
// TestHandleResetFromNotSentRetriesExhausted:
handler := HandleReset(store, &stubEnqueuer{})
```

**Step 2: Run tests to verify compilation fails**

Run: `go test -run TestHandleReset ./internal/infra/httpapi/...`
Expected: FAIL — `too many arguments in call to HandleReset`

**Step 3: Update HandleReset signature and add enqueue logic**

In `internal/infra/httpapi/reset.go`, add the `enqueuer` parameter and
enqueue after the successful `UpdateNotification`:

```go
import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/queue"
)

// HandleReset returns an http.HandlerFunc for POST /v1/notify/reset.
func HandleReset(store domain.ResetStore, enqueuer domain.Enqueuer) http.HandlerFunc {
```

After the `store.UpdateNotification` call and before writing 204,
add the enqueue block (mirrors HandleNotify):

```go
		// REQ-007: Enqueue a delivery job so the worker re-attempts delivery.
		reqID := RequestIDFromContext(r.Context())
		jobPayload := queue.EmailJobPayload{
			NotificationID: n.ID,
			Email:          n.Email,
			RequestID:      reqID,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(r.Context(), payload); err != nil {
			slog.Error("enqueue reset delivery job failed",
				"error", err,
				"notification_id", n.ID,
			)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// REQ-007a: 204 No Content with empty body.
		w.WriteHeader(http.StatusNoContent)
```

**Step 4: Update daemon wiring**

In `cmd/daemon/daemon.go:216`, pass the enqueuer (`nq`) to `HandleReset`:

```go
mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store, nq))
```

**Step 5: Run tests to verify they pass**

Run: `go test -run TestHandleReset ./internal/infra/httpapi/...`
Expected: PASS (all six reset tests)

**Step 6: Commit**

`fix(httpapi): enqueue delivery job after reset (REQ-007)`

### Task 2: Add enqueuer to MCP HandleResetNotification

**Files:**
- Modify: `internal/infra/mcp/tools.go:69` (signature)
- Modify: `internal/infra/mcp/tools.go:108-112` (add enqueue after update)
- Modify: `internal/infra/mcp/server.go:48` (pass enqueuer)
- Modify: `internal/infra/mcp/tools_test.go` (update calls, add assertion)

**Step 1: Write the failing test — assert enqueue on MCP reset**

In `internal/infra/mcp/tools_test.go`, update `TestResetNotificationTool`
to pass a `stubEnqueuer` and assert a job was enqueued:

```go
func TestResetNotificationTool(t *testing.T) {
	store := newStubStore()
	store.notifications["user@company.com"] = &domain.Notification{
		ID:         "ntf_mcp-reset-1",
		Email:      "user@company.com",
		Status:     domain.StatusDelivered,
		RetryCount: 2,
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

	// REQ-007: Verify a delivery job was enqueued.
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(enqueuer.jobs))
	}
}
```

Also update every other test that calls `HandleResetNotification(store)`
to pass `&stubEnqueuer{}`:

- `TestResetNotificationToolNotFound`
- `TestHandleResetNotificationRetriesRemaining`
- `TestHandleResetNotificationRetriesExhausted`
- `TestResetNotificationToolInternalErrorSanitized`
- `TestResetNotificationToolStateMachineErrorSanitized`
- `TestResetNotificationToolUpdateErrorSanitized`

**Step 2: Run tests to verify compilation fails**

Run: `go test -run TestResetNotification ./internal/infra/mcp/...`
Expected: FAIL — `too many arguments in call to HandleResetNotification`

**Step 3: Update HandleResetNotification signature and add enqueue logic**

In `internal/infra/mcp/tools.go`, change the signature:

```go
func HandleResetNotification(store domain.ResetStore, enqueuer domain.Enqueuer) server.ToolHandlerFunc {
```

After the `store.UpdateNotification` call, add the enqueue block:

```go
		// REQ-007: Enqueue a delivery job so the worker re-attempts delivery.
		jobPayload := queue.EmailJobPayload{
			NotificationID: n.ID,
			Email:          n.Email,
		}
		payload, _ := json.Marshal(jobPayload)
		if err := enqueuer.Enqueue(ctx, payload); err != nil {
			slog.Error("enqueue reset delivery job failed",
				"error", err,
				"notification_id", n.ID,
			)
			return gomcp.NewToolResultError("internal error"), nil
		}

		return gomcp.NewToolResultText("notification reset"), nil
```

**Step 4: Update MCP server wiring**

In `internal/infra/mcp/server.go:48`, pass the enqueuer:

```go
HandleResetNotification(store, enqueuer),
```

**Step 5: Run tests to verify they pass**

Run: `go test -run TestResetNotification ./internal/infra/mcp/...`
Expected: PASS (all reset-related MCP tests)

Also run the full test suite to confirm nothing else broke:

Run: `mise run test:unit`
Expected: PASS

**Step 6: Commit**

`fix(mcp): enqueue delivery job after reset (REQ-007)`

### Task 3: Fix Resend button min-width

**Files:**
- Modify: `web/src/components/NotificationRow.tsx:57`

**Step 1: Update the min-width class**

In `web/src/components/NotificationRow.tsx`, change line 57:

```tsx
className="min-w-[7.5rem]"
```

This changes from `7rem` (112px) to `7.5rem` (120px), which
accommodates "Resending..." (115.3px) with margin. Satisfies REQ-062.

**Step 2: Visual verification**

Run the dev server and confirm:
1. The Resend button does not change width when toggling between
   "Resend" and "Resending..." states.
2. The Actions column width remains stable.

Run: `mise run dev`
Expected: No layout shift on resend click.

**Step 3: Commit**

`fix(web): widen Resend button min-width to fit "Resending..." (REQ-062)`

## Verification Checklist

- [ ] `mise run build:go` succeeds with no warnings
- [ ] `mise run test:unit` — all tests pass
- [ ] `mise run lint:go` — no warnings
- [ ] `mise run lint:web` — no warnings
- [ ] Manual: reset a delivered notification, confirm it transitions to pending AND the worker picks it up and re-sends the email (check Mailpit)
- [ ] Manual: resend button has no layout shift between "Resend" and "Resending..."
