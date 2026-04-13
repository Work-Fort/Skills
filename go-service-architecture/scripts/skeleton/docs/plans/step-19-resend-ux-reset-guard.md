---
type: plan
step: "19"
title: "Resend Button UX and Reset Guard"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: 17
  roadmap_step: "19"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-8-dashboard
  - step-7-frontend-foundation
  - step-5-reset-list-postgres
  - step-6-mcp-and-websocket
---

# Step 19: Resend Button UX and Reset Guard

## Overview

When a notification is in `not_sent` state with retries remaining, the
system will automatically retry delivery. Allowing a manual reset during
active retry creates a race condition and wastes retries. This step adds
a domain-layer guard that rejects reset attempts while auto-retry is
still in progress, and updates the frontend to disable the Resend button
in that state.

After this step:

- A new `ErrRetriesRemaining` sentinel error in the domain layer
  signals that a reset was rejected because retries are still in
  progress.
- The REST reset handler (`POST /v1/notify/reset`) checks `not_sent`
  notifications for `retry_count < retry_limit` before firing
  `TriggerReset` and returns 409 Conflict when retries remain.
- The MCP `reset_notification` tool mirrors the same guard, returning a
  tool error with `"notification has retries remaining"`.
- The `NotificationRow` component disables the Resend button when
  `status === 'not_sent' && retry_count < retry_limit`.
- A new Storybook story variant shows the disabled-during-retry state.

## Prerequisites

- Step 8 completed: dashboard with `NotificationRow`, `useNotifications`
  hook, REST reset endpoint, and MCP reset tool all working.

## Spec Traceability

| Task | Spec Requirements |
|------|-------------------|
| Task 1 | notification-management REQ-023, REQ-024, REQ-025 |
| Task 2 | notification-management REQ-023 |
| Task 3 | mcp-integration REQ-017, REQ-018 |
| Task 4 | frontend-dashboard REQ-064, REQ-065, REQ-066, REQ-067 |
| Task 5 | frontend-dashboard REQ-065 |

## Tasks

### Task 1: Add ErrRetriesRemaining Sentinel and Domain Guard Logic

**Files:**
- Modify: `internal/domain/errors.go:5-16`
- Create: `internal/domain/reset_guard.go`
- Create: `internal/domain/reset_guard_test.go`

**Step 1: Write the failing test**

Create `internal/domain/reset_guard_test.go`:

```go
package domain

import (
	"errors"
	"testing"
)

func TestCheckResetAllowed(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		retryCount int
		retryLimit int
		wantErr    error
	}{
		{
			name:       "not_sent with retries remaining is rejected",
			status:     StatusNotSent,
			retryCount: 1,
			retryLimit: 3,
			wantErr:    ErrRetriesRemaining,
		},
		{
			name:       "not_sent with retries exhausted is allowed",
			status:     StatusNotSent,
			retryCount: 3,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "failed is always allowed",
			status:     StatusFailed,
			retryCount: 1,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "delivered is always allowed",
			status:     StatusDelivered,
			retryCount: 0,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "not_sent with zero retries remaining (edge: 0/0)",
			status:     StatusNotSent,
			retryCount: 0,
			retryLimit: 0,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckResetAllowed(tt.status, tt.retryCount, tt.retryLimit)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckResetAllowed() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestCheckResetAllowed ./internal/domain/...`
Expected: FAIL with "undefined: CheckResetAllowed" and "undefined: ErrRetriesRemaining"

**Step 3: Add the sentinel error**

In `internal/domain/errors.go`, add `ErrRetriesRemaining` to the var block:

```go
// ErrRetriesRemaining is returned when a reset is attempted on a
// notification that still has automatic retries in progress.
ErrRetriesRemaining = errors.New("notification has retries remaining")
```

**Step 4: Write the guard function**

Create `internal/domain/reset_guard.go`:

```go
package domain

// CheckResetAllowed returns ErrRetriesRemaining if the notification is
// in not_sent state and retry_count < retry_limit (auto-retry still in
// progress). For failed or delivered notifications, reset is always
// allowed. This centralises the guard logic so both REST and MCP
// handlers use the same check (REQ-023, REQ-024, REQ-025).
func CheckResetAllowed(status Status, retryCount, retryLimit int) error {
	if status == StatusNotSent && retryCount < retryLimit {
		return ErrRetriesRemaining
	}
	return nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test -run TestCheckResetAllowed ./internal/domain/...`
Expected: PASS

**Step 6: Commit**

`feat(domain): add ErrRetriesRemaining sentinel and CheckResetAllowed guard`

### Task 2: Wire Reset Guard into REST Handler

**Depends on:** Task 1

**Files:**
- Modify: `internal/infra/httpapi/reset.go:35-67`
- Modify: `internal/infra/httpapi/reset_test.go`

**Step 1: Write the failing test for 409 Conflict**

Add to `internal/infra/httpapi/reset_test.go`:

```go
func TestHandleResetRetriesRemaining(t *testing.T) {
	store := newResetStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_reset-guard",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleReset(store)

	body := `{"email": "retry@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "notification has retries remaining" {
		t.Errorf("error = %q, want %q", resp["error"], "notification has retries remaining")
	}
}
```

**Step 2: Update the existing `TestHandleResetFromNotSent` test**

The existing test at line 147 uses `RetryCount: 1` with `RetryLimit: DefaultRetryLimit` (3), meaning retries are still remaining. With the new guard this should now return 409. Update it to test the retries-exhausted case instead:

```go
func TestHandleResetFromNotSentRetriesExhausted(t *testing.T) {
	store := newResetStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_reset-2",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleReset(store)

	body := `{"email": "retry@company.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/notify/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	n := store.notifications["retry@company.com"]
	if n.Status != domain.StatusPending {
		t.Errorf("status = %v, want %v", n.Status, domain.StatusPending)
	}
	if n.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", n.RetryCount)
	}
}
```

**Step 3: Run tests to verify the new test fails**

Run: `go test -run TestHandleResetRetriesRemaining ./internal/infra/httpapi/...`
Expected: FAIL -- currently returns 204, not 409

**Step 4: Add the guard check to HandleReset**

In `internal/infra/httpapi/reset.go`, insert the guard check after the
`store.GetNotificationByEmail` block (after the existing not-found
check, before the state machine block). Add between lines 49 and 51:

```go
		// REQ-023: Reject reset when auto-retry is still in progress.
		if err := domain.CheckResetAllowed(n.Status, n.RetryCount, n.RetryLimit); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": err.Error(),
			})
			return
		}
```

**Step 5: Run all reset tests to verify they pass**

Run: `go test -run TestHandleReset ./internal/infra/httpapi/...`
Expected: PASS (all six tests including the renamed one)

**Step 6: Commit**

`feat(httpapi): reject reset with 409 when notification has retries remaining`

### Task 3: Wire Reset Guard into MCP Handler

**Depends on:** Task 1

**Files:**
- Modify: `internal/infra/mcp/tools.go:69-110`
- Modify: `internal/infra/mcp/tools_test.go`

**Step 1: Write the failing test**

Add to `internal/infra/mcp/tools_test.go`:

```go
func TestHandleResetNotificationRetriesRemaining(t *testing.T) {
	store := newStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_mcp-guard",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 1,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleResetNotification(store)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "retry@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error result")
	}

	text := result.Content[0].(gomcp.TextContent).Text
	if text != "notification has retries remaining" {
		t.Errorf("text = %q, want %q", text, "notification has retries remaining")
	}
}

func TestHandleResetNotificationRetriesExhausted(t *testing.T) {
	store := newStubStore()
	store.notifications["retry@company.com"] = &domain.Notification{
		ID:         "ntf_mcp-exhausted",
		Email:      "retry@company.com",
		Status:     domain.StatusNotSent,
		RetryCount: 3,
		RetryLimit: domain.DefaultRetryLimit,
	}

	handler := HandleResetNotification(store)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"email": "retry@company.com"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}
}
```

**Step 2: Run test to verify the guard test fails**

Run: `go test -run TestHandleResetNotificationRetriesRemaining ./internal/infra/mcp/...`
Expected: FAIL -- currently succeeds (no guard check)

**Step 3: Add the guard check to HandleResetNotification**

In `internal/infra/mcp/tools.go`, insert the guard check in
`HandleResetNotification` after the not-found error check (after line
83), before the state machine block:

```go
		// REQ-017: Reject reset when auto-retry is still in progress.
		if err := domain.CheckResetAllowed(n.Status, n.RetryCount, n.RetryLimit); err != nil {
			return gomcp.NewToolResultError(err.Error()), nil
		}
```

**Step 4: Run all MCP reset tests to verify they pass**

Run: `go test -run TestHandleResetNotification ./internal/infra/mcp/...`
Expected: PASS

**Step 5: Commit**

`feat(mcp): reject reset_notification when retries remaining`

### Task 4: Disable Resend Button During Active Retry

**Depends on:** None (frontend-only)

**Files:**
- Modify: `web/src/components/NotificationRow.tsx:30-54`

**Step 1: Update NotificationRow to compute disabled state**

In `web/src/components/NotificationRow.tsx`, replace the existing
disabled logic. Change the button's `disabled` prop from just
`resending` to also account for the retry guard (REQ-067):

Replace the existing `showResend` and button block (lines 30-57):

```tsx
  // REQ-064/REQ-066: Resend visible for failed and not_sent states.
  const showResend = onResend && resendableStates.includes(status)

  // REQ-065/REQ-067: Disable when resending OR when auto-retry is
  // still in progress (not_sent with retries remaining).
  const retryInProgress = status === 'not_sent' && retry_count < retry_limit
  const disableResend = resending || retryInProgress
```

And update the Button's `disabled` prop from `disabled={resending}` to
`disabled={disableResend}`:

```tsx
        {showResend && (
          <Button
            variant="secondary"
            className="min-w-[7rem]"
            onClick={() => onResend(id)}
            disabled={disableResend}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
```

**Step 2: Verify lint passes**

Run: `mise run lint:web`
Expected: PASS with no errors

**Step 3: Commit**

`feat(web): disable resend button while auto-retry in progress`

### Task 5: Add Storybook Story for Disabled-During-Retry State

**Depends on:** Task 4

**Files:**
- Modify: `web/src/components/NotificationRow.stories.tsx`

**Step 1: Add the new story variant**

Add after the existing `NotSent` story in
`web/src/components/NotificationRow.stories.tsx`:

```tsx
export const NotSentRetryInProgress: Story = {
  args: {
    notification: {
      id: 'ntf_retry01',
      email: 'retrying@company.com',
      status: 'not_sent',
      retry_count: 1,
      retry_limit: 3,
    },
  },
}

export const NotSentRetriesExhausted: Story = {
  args: {
    notification: {
      id: 'ntf_exhausted01',
      email: 'exhausted@company.com',
      status: 'not_sent',
      retry_count: 3,
      retry_limit: 3,
    },
  },
}
```

**Step 2: Verify Storybook builds**

Run: `mise run build:web`
Expected: PASS

**Step 3: Commit**

`feat(web): add storybook stories for retry-in-progress disabled state`

### Task 6: Full Build and Lint Verification

**Depends on:** Tasks 1-5

**Files:** None (verification only)

**Step 1: Run Go tests**

Run: `mise run test:unit`
Expected: PASS -- all tests including new guard tests

**Step 2: Run Go linter**

Run: `mise run lint:go`
Expected: PASS with no warnings

**Step 3: Run frontend lint**

Run: `mise run lint:web`
Expected: PASS with no errors

**Step 4: Run frontend build**

Run: `mise run build:web`
Expected: PASS -- dist bundle produced

**Step 5: Commit (if any fixups needed)**

`fix: address lint/build issues from step 19`

## Verification Checklist

- [ ] `mise run build:go` succeeds
- [ ] `mise run test:unit` passes (including new guard tests)
- [ ] `mise run lint:go` produces no warnings
- [ ] `mise run lint:web` produces no errors
- [ ] `mise run build:web` produces working bundle
- [ ] POST /v1/notify/reset with not_sent + retries remaining returns 409
- [ ] POST /v1/notify/reset with not_sent + retries exhausted returns 204
- [ ] POST /v1/notify/reset with failed returns 204 (unchanged)
- [ ] MCP reset_notification mirrors same guard behavior
- [ ] Storybook shows disabled Resend button for NotSentRetryInProgress
- [ ] Storybook shows enabled Resend button for NotSentRetriesExhausted
- [ ] Storybook shows enabled Resend button for Failed (unchanged)
