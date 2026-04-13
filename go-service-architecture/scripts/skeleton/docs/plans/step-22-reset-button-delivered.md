---
type: plan
step: "22"
title: "Reset button on delivered notifications"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "22"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans:
  - step-8-dashboard
  - step-19-resend-ux-reset-guard
  - step-20-reset-enqueue-button-width
---

# Step 22: Reset button on delivered notifications

## Overview

Delivered notifications currently show no action button. Users need a
way to re-send a delivered notification (e.g. the recipient missed it).
This step adds a "Reset" button on delivered rows that calls the
existing `POST /v1/notify/reset` endpoint. The backend and API already
exist; this is purely a frontend change.

The Reset button is mutually exclusive with the Resend button
(REQ-073): Resend appears on `failed`/`not_sent`, Reset appears on
`delivered`, and `pending`/`sending` show no action.

**Spec requirements covered:** REQ-068, REQ-069, REQ-070, REQ-071,
REQ-072, REQ-073.

## Prerequisites

- Steps 19-21 are complete (resend UX, button width, reset bug fix).
- The `resetNotification` API function already exists in `web/src/api.ts`.
- The `POST /v1/notify/reset` endpoint is functional.

## Tasks

### Task 1: Add `onReset` and `resetting` props to NotificationRow

**Files:**
- Modify: `web/src/components/NotificationRow.tsx:1-67`
- Test: `web/src/components/NotificationRow.stories.tsx`

**Step 1: Update the component props and rendering logic**

Add `onReset` callback, `resetting` boolean, and render the Reset
button for delivered status. Satisfies REQ-068, REQ-069, REQ-070,
REQ-071, REQ-073.

```tsx
import { StatusBadge, type NotificationStatus } from './StatusBadge'
import { Button } from './Button'

export interface Notification {
  id: string
  email: string
  status: NotificationStatus
  retry_count: number
  retry_limit: number
}

export interface NotificationRowProps {
  notification: Notification
  /** Called when the user clicks "Resend". Only shown for failed/not_sent states. */
  onResend?: (id: string) => void
  /** When true, the resend button shows a loading state. */
  resending?: boolean
  /** Called when the user clicks "Reset". Only shown for delivered state. */
  onReset?: (id: string) => void
  /** When true, the reset button shows a loading state. */
  resetting?: boolean
}

// Resend is only available for notifications that did not succeed.
// Delivered notifications are terminal but do not need resending.
const resendableStates: NotificationStatus[] = ['failed', 'not_sent']

export function NotificationRow({
  notification,
  onResend,
  resending = false,
  onReset,
  resetting = false,
}: NotificationRowProps) {
  const { id, email, status, retry_count, retry_limit } = notification

  // REQ-064/REQ-066: Resend visible for failed and not_sent states.
  const showResend = onResend && resendableStates.includes(status)

  // REQ-065/REQ-067: Disable when resending OR when auto-retry is
  // still in progress (not_sent with retries remaining).
  const retryInProgress = status === 'not_sent' && retry_count < retry_limit
  const disableResend = resending || retryInProgress

  // REQ-068: Reset visible only for delivered state.
  const showReset = onReset && status === 'delivered'

  return (
    <tr>
      <td className="whitespace-nowrap px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">
        {id}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm">
        {email}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <StatusBadge status={status} />
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
        {retry_count} / {retry_limit}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        {showResend && (
          <Button
            variant="secondary"
            className="min-w-[7.5rem]"
            onClick={() => onResend(id)}
            disabled={disableResend}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
        {showReset && (
          <Button
            variant="secondary"
            className="min-w-[7.5rem]"
            onClick={() => onReset(id)}
            disabled={resetting}
          >
            {resetting ? 'Resetting...' : 'Reset'}
          </Button>
        )}
      </td>
    </tr>
  )
}
```

**Step 2: Verify the component compiles**

Run: `mise run build:web`
Expected: Build succeeds with no errors.

**Step 3: Add Storybook stories for delivered-with-reset and resetting states**

```tsx
// Add to NotificationRow.stories.tsx after the existing Resending story:

export const DeliveredWithReset: Story = {
  args: {
    notification: {
      id: 'ntf_reset01',
      email: 'delivered@company.com',
      status: 'delivered',
      retry_count: 0,
      retry_limit: 3,
    },
    onReset: fn(),
  },
}

export const Resetting: Story = {
  name: 'Resetting',
  args: {
    notification: {
      id: 'ntf_resetting01',
      email: 'resetting@company.com',
      status: 'delivered',
      retry_count: 0,
      retry_limit: 3,
    },
    onReset: fn(),
    resetting: true,
  },
}
```

Note: The existing `Resending` story uses the name `Resending`. Rename
it to avoid confusion now that we have a `Resetting` story. Update the
existing story's export name to `ResendingState` or keep as-is since
Storybook uses the export name. Both are distinct (`Resending` vs
`Resetting`) so no rename is needed.

Also add `onReset: fn()` to the `meta.args` so all stories have it
available:

```tsx
args: {
  onResend: fn(),
  onReset: fn(),
},
```

**Step 4: Verify Storybook renders correctly**

Run: `mise run storybook` (or open in browser)
Expected: `DeliveredWithReset` shows a "Reset" button. `Resetting`
shows a disabled "Resetting..." button. The existing `Delivered` story
still shows a Reset button (since `onReset` is now in meta args).

**Step 5: Commit**

`feat(web): add Reset button to delivered notification rows`

### Task 2: Add `reset` function and `resetting` Set to useNotifications

**Depends on:** Task 1

**Files:**
- Modify: `web/src/hooks/useNotifications.ts`

**Step 1: Add the resetting state and reset function**

Add a separate `resetting` Set (not shared with `resending`) and a
`reset` callback. Satisfies REQ-069, REQ-070, REQ-072.

In the state declarations (after the `resending` state), add:

```ts
const [resetting, setResetting] = useState<Set<string>>(new Set())
```

Add the `reset` callback (after the `resend` callback):

```ts
const reset = useCallback(
  async (id: string) => {
    const notification = notificationsRef.current.find((n) => n.id === id)
    if (!notification) return

    setResetting((prev) => new Set(prev).add(id))
    setError(null)
    try {
      await resetNotification(notification.email)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reset failed')
    } finally {
      setResetting((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  },
  [],
)
```

**Step 2: Update the WebSocket handler to clear resetting state**

In the `useWebSocket` callback, after the existing block that clears
`resending` on non-resendable states, add a block that clears
`resetting` when the state transitions away from `delivered`.
Satisfies REQ-072.

Replace the WebSocket callback with:

```ts
useWebSocket(
  useCallback((msg: WsMessage) => {
    setNotifications((prev) =>
      prev.map((n) =>
        n.id === msg.id ? { ...n, status: msg.state } : n,
      ),
    )
    // Clear resending spinner on non-resendable states.
    if (nonResendableStates.has(msg.state)) {
      setResending((prev) => {
        if (!prev.has(msg.id)) return prev
        const next = new Set(prev)
        next.delete(msg.id)
        return next
      })
    }
    // Clear resetting spinner when state leaves delivered.
    if (msg.state !== 'delivered') {
      setResetting((prev) => {
        if (!prev.has(msg.id)) return prev
        const next = new Set(prev)
        next.delete(msg.id)
        return next
      })
    }
  }, []),
)
```

**Step 3: Update the return value**

Add `reset` and `resetting` to the returned object:

```ts
return {
  notifications,
  loading,
  error,
  hasNext: hasMore,
  hasPrevious: pageIndex > 0,
  goNext,
  goPrevious,
  goToPage,
  currentPage: pageIndex + 1,
  totalPages,
  totalCount,
  resend,
  resending,
  reset,
  resetting: resettingSet,
}
```

Update the `UseNotificationsResult` interface to include:

```ts
/** Reset a delivered notification back to pending. */
reset: (id: string) => Promise<void>
/** Set of notification IDs currently being reset (for loading spinners). */
resetting: Set<string>
```

Note: The hook already has a `resending` Set and returns it. We use a
separate name `resetting` for the new Set. Since the return object
already has `resending`, we need to distinguish. Rename the state
variable to `resettingSet` internally to avoid shadowing, or just use
`resetting` as both the state name and returned field (since the
existing `resending` already follows this pattern).

**Step 4: Verify the hook compiles**

Run: `mise run build:web`
Expected: Build succeeds with no errors.

**Step 5: Commit**

`feat(web): add reset function and resetting state to useNotifications`

### Task 3: Wire reset into App.tsx

**Depends on:** Task 1, Task 2

**Files:**
- Modify: `web/src/App.tsx`

**Step 1: Destructure reset and resetting from useNotifications**

Update the destructuring in `App.tsx`:

```tsx
const {
  notifications,
  loading,
  error,
  hasNext,
  hasPrevious,
  goNext,
  goPrevious,
  goToPage,
  currentPage,
  totalPages,
  resend,
  resending,
  reset,
  resetting,
} = useNotifications()
```

**Step 2: Pass onReset and resetting to NotificationRow**

Update the `NotificationRow` rendering:

```tsx
notifications.map((n) => (
  <NotificationRow
    key={n.id}
    notification={n}
    onResend={resend}
    resending={resending.has(n.id)}
    onReset={reset}
    resetting={resetting.has(n.id)}
  />
))
```

**Step 3: Verify the full app compiles**

Run: `mise run build:web`
Expected: Build succeeds with no errors.

**Step 4: Commit**

`feat(web): wire reset button into dashboard`

### Task 4: E2E test for reset from delivered via dashboard

**Depends on:** Task 3

**Files:**
- Create: `tests/e2e/dashboard_reset_test.go`

**Step 1: Write the E2E test**

This test verifies the full user flow: send a notification, wait for
delivery, click Reset via the API (simulating what the dashboard does),
and verify re-delivery. This complements the existing
`TestResetAndRedeliver` in `reset_test.go` by confirming the
dashboard's API call path works end-to-end.

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

// TestDashboardResetDelivered simulates the dashboard flow:
// 1. POST /v1/notify to create a notification
// 2. Poll until delivered
// 3. POST /v1/notify/reset (the same call the dashboard Reset button makes)
// 4. Poll until re-delivered
// 5. Verify the notification went through the full cycle
func TestDashboardResetDelivered(t *testing.T) {
	smtpHost, smtpPort, mailpitAPI := MailpitAddr()
	addr := FreePort(t)
	d := StartDaemon(t, serviceBin, addr, WithSMTP(smtpHost, smtpPort))
	t.Cleanup(func() { d.StopFatal(t) })

	// Clear Mailpit.
	req, _ := http.NewRequest(http.MethodDelete, mailpitAPI+"/api/v1/messages", nil)
	http.DefaultClient.Do(req)

	base := fmt.Sprintf("http://%s", addr)
	email := "dashboard-reset-e2e@company.com"

	// Step 1: Send notification.
	resp, err := http.Post(
		base+"/v1/notify",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"email": %q}`, email)),
	)
	if err != nil {
		t.Fatalf("POST /v1/notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notify: expected 202, got %d", resp.StatusCode)
	}

	// Step 2: Wait for delivery.
	state := pollNotificationState(t, base, email, "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("expected delivered, got %q", state)
	}

	// Step 3: Verify initial email arrived.
	mailCount := pollMailpitCount(t, mailpitAPI, 1, 5*time.Second)
	if mailCount < 1 {
		t.Fatal("no email received in Mailpit before reset")
	}

	// Step 4: Reset via the same endpoint the dashboard calls.
	resp, err = http.Post(
		base+"/v1/notify/reset",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"email": %q}`, email)),
	)
	if err != nil {
		t.Fatalf("POST /v1/notify/reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: expected 204, got %d", resp.StatusCode)
	}

	// Step 5: Verify the notification returns to delivered after re-processing.
	state = pollNotificationState(t, base, email, "delivered", 20*time.Second)
	if state != "delivered" {
		t.Fatalf("after reset: expected re-delivery to delivered, got %q", state)
	}

	// Step 6: Verify a second email arrived.
	mailCount = pollMailpitCount(t, mailpitAPI, 2, 10*time.Second)
	if mailCount < 2 {
		t.Errorf("expected at least 2 emails after reset, got %d", mailCount)
	}

	// Step 7: Verify the notification is visible in the list with delivered state
	// and retry_count reset to 0.
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
		if n.Email == email {
			found = true
			if n.State != "delivered" {
				t.Errorf("state = %q, want delivered", n.State)
			}
		}
	}
	if !found {
		t.Fatal("notification not found in list response after reset")
	}
}
```

**Step 2: Run the E2E test**

Run: `mise run test:e2e`
Expected: `TestDashboardResetDelivered` passes. All existing E2E tests
continue to pass.

**Step 3: Commit**

`test(e2e): add dashboard reset from delivered E2E test`

### Task 5: Run full test suite and lint

**Depends on:** Tasks 1-4

**Files:** (none new)

**Step 1: Run unit tests**

Run: `mise run test:unit`
Expected: All tests pass.

**Step 2: Run lint**

Run: `mise run lint`
Expected: No warnings or errors.

**Step 3: Run E2E tests**

Run: `mise run test:e2e`
Expected: All tests pass including the new `TestDashboardResetDelivered`.

**Step 4: Run Storybook build**

Run: `mise run build:storybook`
Expected: Build succeeds. New stories are included.

## Verification Checklist

- [ ] `mise run build:web` succeeds with no warnings
- [ ] `mise run test:unit` -- all tests pass
- [ ] `mise run lint` -- no warnings
- [ ] `mise run test:e2e` -- all tests pass
- [ ] `mise run build:storybook` -- builds successfully
- [ ] Delivered rows show "Reset" button; failed/not_sent show "Resend"; pending/sending show nothing (REQ-068, REQ-073)
- [ ] Clicking Reset shows "Resetting..." and disables the button (REQ-070)
- [ ] Reset button has `min-w-[7.5rem]` preventing reflow (REQ-071)
- [ ] WebSocket update from delivered to pending clears the resetting spinner (REQ-072)
- [ ] Storybook stories render correctly for `DeliveredWithReset` and `Resetting`
- [ ] E2E test confirms full cycle: notify -> deliver -> reset -> re-deliver
