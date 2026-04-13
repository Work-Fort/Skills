---
type: plan
step: "18"
title: "Fix resend button layout shift and race condition"
status: pending
assessment_status: needed
provenance:
  source: issue
  issue_id: 20
  roadmap_step: "18"
dates:
  created: "2026-04-12"
  approved: null
  completed: null
related_plans: []
---

# Step 18: Fix Resend Button Layout Shift and Race Condition

## Overview

Two small bugs in the resend button flow:

1. **Layout shift (REQ-061, REQ-062):** The Resend button text toggles between "Resend" (6 chars) and "Resending..." (12 chars). Without a minimum width, the Actions column resizes on every click, causing a jarring table reflow.

2. **Race condition (REQ-063):** When a WebSocket message arrives changing a notification to a non-resendable state (pending, sending, delivered), the `resending` Set is not cleared. This can leave a stale entry that causes a brief render where the button is both disabled and absent.

## Prerequisites

- Step 12 (WebSocket) is merged and the dashboard renders correctly.

## Tasks

### Task 1: Add min-width to Resend button

**Files:**
- Modify: `web/src/components/NotificationRow.tsx:48-54`

**Step 1: Add a min-width class to the Button**

In `NotificationRow.tsx`, add `className="min-w-[6.5rem]"` to the `<Button>` so it reserves enough space for the wider "Resending..." label. The value `6.5rem` (104px at default 16px root) comfortably fits "Resending..." with the existing `px-4 text-sm` padding.

```tsx
        {showResend && (
          <Button
            variant="secondary"
            className="min-w-[6.5rem]"
            onClick={() => onResend(id)}
            disabled={resending}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
```

**Step 2: Verify visually**

Run: `mise run dev:web`

Open the dashboard, click Resend on a failed notification, and confirm the button and Actions column do not change width when the label switches to "Resending...".

**Step 3: Commit**

`fix(web): add min-width to Resend button to prevent layout shift`

Satisfies REQ-061, REQ-062.

### Task 2: Clear resending Set on WebSocket state transition

**Files:**
- Modify: `web/src/hooks/useNotifications.ts:89-97`

**Step 1: Update the WebSocket callback to clear resending IDs**

Replace the existing `useWebSocket` callback in `useNotifications.ts` with one that also removes the notification ID from the `resending` Set when the incoming state is non-resendable (i.e., `pending`, `sending`, or `delivered`):

```ts
  const nonResendableStates: Set<NotificationStatus> = new Set([
    'pending',
    'sending',
    'delivered',
  ])

  // WebSocket: merge state updates into the current list.
  useWebSocket(
    useCallback((msg: WsMessage) => {
      setNotifications((prev) =>
        prev.map((n) =>
          n.id === msg.id ? { ...n, status: msg.state } : n,
        ),
      )
      if (nonResendableStates.has(msg.state)) {
        setResending((prev) => {
          if (!prev.has(msg.id)) return prev
          const next = new Set(prev)
          next.delete(msg.id)
          return next
        })
      }
    }, []),
  )
```

The `nonResendableStates` set is declared outside the callback (but inside the hook function body) so it is allocated once per hook instance. The guard `if (!prev.has(msg.id)) return prev` avoids creating a new Set when nothing changed.

Note: `NotificationStatus` must be imported. It is already available transitively through the `Notification` type from `'../components'`, but the WebSocket callback references it directly. Add the import:

```ts
import type { NotificationStatus } from '../components'
```

**Step 2: Verify the fix**

Run: `mise run dev:web`

1. Click Resend on a failed notification.
2. Observe the button shows "Resending..." (disabled).
3. When the WebSocket delivers the `pending` state update, the button should disappear cleanly without a flash of a disabled-but-hidden state.

**Step 3: Commit**

`fix(web): clear resending state on WebSocket non-resendable transition`

Satisfies REQ-063.

## Verification Checklist

- [ ] `mise run build:web` succeeds with no warnings
- [ ] Resend button does not cause column width change when toggling between "Resend" and "Resending..."
- [ ] After clicking Resend, when WebSocket delivers pending/sending/delivered, the resending Set is cleared and no stale spinner remains
- [ ] Dark mode rendering is unaffected
