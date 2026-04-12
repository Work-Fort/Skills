---
type: plan
step: "8"
title: "Dashboard"
status: pending
assessment_status: in_progress
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "8"
dates:
  created: "2026-04-10"
  approved: null
  completed: null
related_plans:
  - step-7-frontend-foundation
  - step-6-mcp-and-websocket
  - step-5-reset-list-postgres
---

# Step 8: Dashboard

## Overview

Assemble the working Notifier dashboard by connecting the reusable
components from Step 7 to the real REST API and WebSocket endpoint.
This is the final step: after it, the SPA loads notifications from
`GET /v1/notifications`, displays them in a paginated table, updates
in real time via the `/v1/ws` WebSocket, and supports resending failed
notifications by calling the reset endpoint (which transitions back to
`pending` for automatic re-enqueueing by the queue worker).

After this step:

- A typed API client module (`web/src/api.ts`) wraps `fetch` calls to
  the list, notify, and reset endpoints with typed request/response
  interfaces.
- A `useWebSocket` hook (`web/src/hooks/useWebSocket.ts`) connects to
  `/v1/ws`, parses incoming state-change messages, and exposes them to
  consuming components.
- A `useNotifications` hook (`web/src/hooks/useNotifications.ts`)
  manages the notification list state, pagination cursors, loading
  states, and merges WebSocket updates into the list.
- The `App.tsx` dashboard page composes `DarkModeToggle`,
  `NotificationRow`, and `Pagination` into the full UI with a header,
  table, and pagination controls.
- A resend button on each failed/not_sent row calls
  `POST /v1/notify/reset`, which transitions the notification back to
  `pending` so the queue worker automatically re-enqueues it for
  delivery. A per-row loading spinner indicates the operation is in
  progress.
- A Storybook story for the assembled dashboard exists with mocked
  data, demonstrating the full layout in both light and dark modes.
- `mise run lint:web` passes with no type errors.
- `mise run build:web` produces a working `web/dist/` bundle.

## Prerequisites

- Step 7 completed: all five reusable components (`Button`,
  `StatusBadge`, `Pagination`, `DarkModeToggle`, `NotificationRow`)
  exist with Storybook stories, Go embed files and SPA handler are
  wired, dev proxy works.
- Step 6 completed: WebSocket hub broadcasts `{"id":"...","state":"..."}`
  JSON messages on every state transition.
- Step 5 completed: `GET /v1/notifications` returns paginated list with
  cursor-based pagination; `POST /v1/notify/reset` resets a notification.
- Step 3 completed: `POST /v1/notify` sends a notification.
- Node 22 and `mise` CLI available on PATH.

## New Dependencies

None. All npm packages were installed in Step 7. All code in this step
uses `fetch` (built into the browser), the native `WebSocket` API, and
React hooks from the existing `react` dependency.

## Spec Traceability

All tasks trace to `openspec/specs/frontend-dashboard/spec.md`:

| Task | Spec Requirements |
|------|-------------------|
| Task 1: API client module | REQ-015 |
| Task 2: useWebSocket hook | REQ-013 |
| Task 3: useNotifications hook | REQ-012, REQ-013, REQ-014 |
| Task 4: Dashboard page | REQ-012, REQ-014, REQ-017 |
| Task 5: Dashboard story | REQ-020 |
| Task 6: Type-check and build | REQ-001 |

## Tasks

### Task 1: API Client Module

Satisfies: REQ-015 (API client module for communicating with REST
endpoints).

**Files:**
- Create: `web/src/api.ts`

**Step 1: Create the API client module**

```typescript
import type { NotificationStatus } from './components'

// -- Response types matching the Go JSON responses --

export interface ListNotification {
  id: string
  email: string
  state: NotificationStatus
  retry_count: number
  retry_limit: number
  created_at: string
  updated_at: string
}

export interface ListMeta {
  has_more: boolean
  next_cursor?: string
}

export interface ListResponse {
  notifications: ListNotification[]
  meta: ListMeta
}

export interface NotifyResponse {
  id: string
}

export interface ApiError {
  error: string
}

// -- API functions --

const BASE = '/v1'

/**
 * Fetch a page of notifications. Pass `cursor` for subsequent pages.
 */
export async function fetchNotifications(
  limit = 20,
  cursor?: string,
): Promise<ListResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('after', cursor)
  const res = await fetch(`${BASE}/notifications?${params}`)
  if (!res.ok) {
    const body: ApiError = await res.json()
    throw new Error(body.error)
  }
  return res.json()
}

/**
 * Reset a notification by email so it can be re-sent.
 * Returns void on success (204 No Content).
 */
export async function resetNotification(email: string): Promise<void> {
  const res = await fetch(`${BASE}/notify/reset`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const body: ApiError = await res.json()
    throw new Error(body.error)
  }
}

/**
 * Send a notification to an email address.
 * Returns the notification ID on success (202 Accepted).
 */
export async function sendNotification(
  email: string,
): Promise<NotifyResponse> {
  const res = await fetch(`${BASE}/notify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const body: ApiError = await res.json()
    throw new Error(body.error)
  }
  return res.json()
}
```

**Step 2: Verify types compile**

Run: `cd web && npx tsc -b`
Expected: PASS with no errors

**Step 3: Commit**

`feat(web): add typed API client module`

### Task 2: useWebSocket Hook

Satisfies: REQ-013 (connect to `/v1/ws` for real-time state updates
without polling).

**Files:**
- Create: `web/src/hooks/useWebSocket.ts`

**Step 1: Create the useWebSocket hook**

The WebSocket server broadcasts JSON messages in the format
`{"id": "ntf_...", "state": "delivered"}` whenever a notification
transitions. This hook connects, parses messages, and invokes a
callback.

```typescript
import { useEffect, useRef } from 'react'
import type { NotificationStatus } from '../components'

export interface WsMessage {
  id: string
  state: NotificationStatus
}

/**
 * Connects to the WebSocket endpoint and calls `onMessage` for each
 * state-change event. Reconnects automatically on disconnect with
 * exponential backoff (1s, 2s, 4s, capped at 30s).
 */
export function useWebSocket(onMessage: (msg: WsMessage) => void) {
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  useEffect(() => {
    let ws: WebSocket | null = null
    let delay = 1000
    let timer: ReturnType<typeof setTimeout> | null = null
    let stopped = false

    function connect() {
      if (stopped) return

      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
      ws = new WebSocket(`${proto}//${location.host}/v1/ws`)

      ws.onopen = () => {
        delay = 1000 // reset backoff on successful connect
      }

      ws.onmessage = (event) => {
        try {
          const msg: WsMessage = JSON.parse(event.data)
          onMessageRef.current(msg)
        } catch {
          // ignore malformed messages
        }
      }

      ws.onclose = () => {
        if (stopped) return
        timer = setTimeout(() => {
          delay = Math.min(delay * 2, 30000)
          connect()
        }, delay)
      }

      ws.onerror = () => {
        ws?.close()
      }
    }

    connect()

    return () => {
      stopped = true
      if (timer) clearTimeout(timer)
      ws?.close()
    }
  }, [])
}
```

**Step 2: Verify types compile**

Run: `cd web && npx tsc -b`
Expected: PASS with no errors

**Step 3: Commit**

`feat(web): add useWebSocket hook with auto-reconnect`

### Task 3: useNotifications Hook

Satisfies: REQ-012 (display table of all notifications), REQ-013
(real-time updates via WebSocket), REQ-014 (resend button).

This hook manages the full notification list state: initial fetch,
pagination cursors, WebSocket-driven state updates, and the resend
flow (reset to `pending`, after which the queue worker automatically
re-enqueues).

**Depends on:** Task 1 (API client), Task 2 (useWebSocket hook)

**Files:**
- Create: `web/src/hooks/useNotifications.ts`

**Step 1: Create the useNotifications hook**

```typescript
import { useState, useEffect, useCallback, useRef } from 'react'
import type { Notification } from '../components'
import { fetchNotifications, resetNotification } from '../api'
import { useWebSocket, type WsMessage } from './useWebSocket'

interface UseNotificationsResult {
  /** The current page of notifications. */
  notifications: Notification[]
  /** True while the initial fetch is in progress. */
  loading: boolean
  /** Error message from the last failed operation, or null. */
  error: string | null
  /** True if there is a next page. */
  hasNext: boolean
  /** True if there is a previous page (not on the first page). */
  hasPrevious: boolean
  /** Load the next page. */
  goNext: () => void
  /** Load the previous page. */
  goPrevious: () => void
  /** Reset a notification to pending so the queue worker re-enqueues it. */
  resend: (id: string) => Promise<void>
  /** Set of notification IDs currently being resent (for loading spinners). */
  resending: Set<string>
}

const PAGE_SIZE = 20

export function useNotifications(): UseNotificationsResult {
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [resending, setResending] = useState<Set<string>>(new Set())

  // Cursor stack for pagination: each entry is the cursor that
  // produced the current page. Index 0 = first page (no cursor).
  const [cursorStack, setCursorStack] = useState<(string | undefined)[]>([
    undefined,
  ])
  const [pageIndex, setPageIndex] = useState(0)
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [hasMore, setHasMore] = useState(false)

  // Keep a ref to the current notifications for the WS callback.
  const notificationsRef = useRef(notifications)
  notificationsRef.current = notifications

  // Fetch a page by cursor.
  const fetchPage = useCallback(async (cursor?: string) => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchNotifications(PAGE_SIZE, cursor)
      const items: Notification[] = data.notifications.map((n) => ({
        id: n.id,
        email: n.email,
        status: n.state,
        retry_count: n.retry_count,
        retry_limit: n.retry_limit,
      }))
      setNotifications(items)
      setHasMore(data.meta.has_more)
      setNextCursor(data.meta.next_cursor)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [])

  // Initial fetch.
  useEffect(() => {
    fetchPage()
  }, [fetchPage])

  // WebSocket: merge state updates into the current list.
  useWebSocket(
    useCallback((msg: WsMessage) => {
      setNotifications((prev) =>
        prev.map((n) =>
          n.id === msg.id ? { ...n, status: msg.state } : n,
        ),
      )
    }, []),
  )

  const goNext = useCallback(() => {
    if (!hasMore || !nextCursor) return
    const newStack = [...cursorStack]
    if (pageIndex + 1 >= newStack.length) {
      newStack.push(nextCursor)
    }
    setCursorStack(newStack)
    setPageIndex((i) => i + 1)
    fetchPage(nextCursor)
  }, [hasMore, nextCursor, cursorStack, pageIndex, fetchPage])

  const goPrevious = useCallback(() => {
    if (pageIndex <= 0) return
    const newIndex = pageIndex - 1
    setPageIndex(newIndex)
    fetchPage(cursorStack[newIndex])
  }, [pageIndex, cursorStack, fetchPage])

  const resend = useCallback(
    async (id: string) => {
      const notification = notificationsRef.current.find((n) => n.id === id)
      if (!notification) return

      setResending((prev) => new Set(prev).add(id))
      setError(null)
      try {
        // Reset transitions the notification back to pending. The
        // queue worker automatically re-enqueues it for delivery.
        // No separate POST /v1/notify call is needed (and would fail
        // with 409 since the record still exists).
        await resetNotification(notification.email)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Resend failed')
      } finally {
        setResending((prev) => {
          const next = new Set(prev)
          next.delete(id)
          return next
        })
      }
    },
    [],
  )

  return {
    notifications,
    loading,
    error,
    hasNext: hasMore,
    hasPrevious: pageIndex > 0,
    goNext,
    goPrevious,
    resend,
    resending,
  }
}
```

**Step 2: Verify types compile**

Run: `cd web && npx tsc -b`
Expected: PASS with no errors

**Step 3: Commit**

`feat(web): add useNotifications hook with pagination and resend`

### Task 4: Dashboard Page

Satisfies: REQ-012 (notifications table with state, email, retry
count/limit), REQ-014 (resend button), REQ-017 (dark mode support via
existing Tailwind classes).

**Depends on:** Task 3 (useNotifications hook)

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/NotificationRow.tsx`
- Modify: `web/src/components/NotificationRow.stories.tsx`

**Step 1: Replace App.tsx with the dashboard page**

Replace the placeholder content in `App.tsx` with the full dashboard
layout. The page composes `DarkModeToggle`, `NotificationRow`, and
`Pagination` around the `useNotifications` hook.

```typescript
import {
  DarkModeToggle,
  NotificationRow,
  Pagination,
  Button,
} from './components'
import { useNotifications } from './hooks/useNotifications'

function App() {
  const {
    notifications,
    loading,
    error,
    hasNext,
    hasPrevious,
    goNext,
    goPrevious,
    resend,
    resending,
  } = useNotifications()

  return (
    <div className="min-h-screen bg-white dark:bg-brand-primary">
      {/* Header */}
      <header className="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
        <div className="mx-auto flex max-w-5xl items-center justify-between">
          <h1 className="text-xl font-bold text-gray-900 dark:text-brand-text">
            Notifier Dashboard
          </h1>
          <DarkModeToggle />
        </div>
      </header>

      {/* Main content */}
      <main className="mx-auto max-w-5xl px-6 py-6">
        {/* Error banner */}
        {error && (
          <div
            className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200"
            role="alert"
          >
            {error}
          </div>
        )}

        {/* Loading state */}
        {loading && notifications.length === 0 ? (
          <div className="flex items-center justify-center py-20">
            <p className="text-gray-500 dark:text-gray-400">
              Loading notifications...
            </p>
          </div>
        ) : notifications.length === 0 ? (
          <div className="flex items-center justify-center py-20">
            <p className="text-gray-500 dark:text-gray-400">
              No notifications yet.
            </p>
          </div>
        ) : (
          <>
            {/* Notifications table */}
            <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
              <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                <thead className="bg-gray-50 dark:bg-brand-surface">
                  <tr>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      ID
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Email
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Status
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Retries
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-brand-primary">
                  {notifications.map((n) => (
                    <NotificationRow
                      key={n.id}
                      notification={n}
                      onResend={resend}
                      resending={resending.has(n.id)}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            <Pagination
              hasPrevious={hasPrevious}
              hasNext={hasNext}
              onPrevious={goPrevious}
              onNext={goNext}
            />
          </>
        )}
      </main>
    </div>
  )
}

export default App
```

**Step 2: Add `resending` prop to NotificationRow**

The resend button needs a loading state. Extend `NotificationRow` to
accept an optional `resending` boolean that disables the button and
shows a loading indicator.

Modify `web/src/components/NotificationRow.tsx` -- replace the full
file content:

```typescript
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
}

// Resend is only available for notifications that did not succeed.
// Delivered notifications are terminal but do not need resending.
const resendableStates: NotificationStatus[] = ['failed', 'not_sent']

export function NotificationRow({
  notification,
  onResend,
  resending = false,
}: NotificationRowProps) {
  const { id, email, status, retry_count, retry_limit } = notification
  const showResend = onResend && resendableStates.includes(status)

  return (
    <tr className="border-b border-gray-200 dark:border-gray-700">
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
            onClick={() => onResend(id)}
            disabled={resending}
          >
            {resending ? 'Resending...' : 'Resend'}
          </Button>
        )}
      </td>
    </tr>
  )
}
```

**Step 3: Update the NotificationRow barrel export to include the new prop**

No change needed -- `NotificationRowProps` is already exported from
`web/src/components/index.ts` and the interface name is unchanged.
TypeScript will pick up the new optional `resending` field
automatically.

**Step 4: Add Resending story variant to NotificationRow**

Modify `web/src/components/NotificationRow.stories.tsx` -- append a
`Resending` variant after the existing `NotSent` export to exercise
the loading spinner state:

```typescript
export const Resending: Story = {
  args: {
    notification: {
      id: 'ntf_resend01',
      email: 'retry@company.com',
      status: 'failed',
      retry_count: 3,
      retry_limit: 3,
    },
    resending: true,
  },
}
```

**Step 5: Verify types compile**

Run: `cd web && npx tsc -b`
Expected: PASS with no errors

**Step 6: Commit**

`feat(web): assemble dashboard page with table, pagination, and resend`

### Task 5: Dashboard Storybook Story

Satisfies: REQ-020 (CSF 3.0 format with `tags: ['autodocs']`).

**Depends on:** Task 4 (dashboard page)

**Files:**
- Create: `web/src/Dashboard.stories.tsx`

**Step 1: Create the dashboard story with mocked data**

The story cannot use the real `App` component directly because it calls
`useNotifications` which hits real endpoints. Instead, create a
`Dashboard` story that renders the same layout with static mock data,
demonstrating the assembled UI in Storybook.

```typescript
import type { Meta, StoryObj } from '@storybook/react'
import {
  DarkModeToggle,
  NotificationRow,
  Pagination,
  type Notification,
} from './components'

const mockNotifications: Notification[] = [
  {
    id: 'ntf_a1b2c3d4',
    email: 'alice@company.com',
    status: 'delivered',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_e5f6g7h8',
    email: 'bob@example.com',
    status: 'failed',
    retry_count: 3,
    retry_limit: 3,
  },
  {
    id: 'ntf_i9j0k1l2',
    email: 'carol@company.com',
    status: 'sending',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_m3n4o5p6',
    email: 'dave@company.com',
    status: 'pending',
    retry_count: 0,
    retry_limit: 3,
  },
  {
    id: 'ntf_q7r8s9t0',
    email: 'eve@company.com',
    status: 'not_sent',
    retry_count: 1,
    retry_limit: 3,
  },
]

function DashboardLayout({
  notifications,
  error,
}: {
  notifications: Notification[]
  error?: string
}) {
  return (
    <div className="min-h-screen bg-white dark:bg-brand-primary">
      <header className="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
        <div className="mx-auto flex max-w-5xl items-center justify-between">
          <h1 className="text-xl font-bold text-gray-900 dark:text-brand-text">
            Notifier Dashboard
          </h1>
          <DarkModeToggle />
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-6">
        {error && (
          <div
            className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200"
            role="alert"
          >
            {error}
          </div>
        )}
        <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-brand-surface">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  ID
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Email
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Status
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Retries
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-brand-primary">
              {notifications.map((n) => (
                <NotificationRow
                  key={n.id}
                  notification={n}
                  onResend={() => {}}
                />
              ))}
            </tbody>
          </table>
        </div>
        <Pagination
          hasPrevious={false}
          hasNext={true}
          onPrevious={() => {}}
          onNext={() => {}}
        />
      </main>
    </div>
  )
}

const meta = {
  component: DashboardLayout,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<typeof DashboardLayout>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    notifications: mockNotifications,
  },
}

export const Empty: Story = {
  args: {
    notifications: [],
  },
}

export const AllDelivered: Story = {
  args: {
    notifications: mockNotifications
      .slice(0, 3)
      .map((n) => ({ ...n, status: 'delivered' as const })),
  },
}

export const AllFailed: Story = {
  args: {
    notifications: mockNotifications
      .slice(0, 3)
      .map((n) => ({
        ...n,
        status: 'failed' as const,
        retry_count: 3,
      })),
  },
}

export const ErrorState: Story = {
  args: {
    notifications: mockNotifications.slice(0, 2),
    error: 'Failed to load notifications',
  },
}
```

**Step 2: Verify the story renders in Storybook**

Run: `cd web && npx storybook build --quiet 2>&1 | tail -5`
Expected: Build completes successfully with no errors

**Step 3: Commit**

`feat(web): add dashboard Storybook story with mocked data`

### Task 6: Type-Check and Build

Satisfies: REQ-001 (build output in `web/dist/`).

**Depends on:** Task 1-5

**Files:**
- No new files

**Step 1: Run the type-checker**

Run: `mise run lint:web`
Expected: PASS with no errors

**Step 2: Build the frontend**

Run: `mise run build:web`
Expected: Build completes and `web/dist/index.html` exists

**Step 3: Verify the Go binary embeds the frontend**

Run: `mise run release:production`
Expected: Build completes with no errors (this task uses `-tags spa`
to include the embedded SPA)

**Step 4: Commit**

No code changes in this task -- this is verification only. If lint or
build revealed errors, they should have been fixed in the tasks above.
No commit needed.

## Verification Checklist

- [ ] `mise run lint:web` passes with no type errors
- [ ] `mise run build:web` produces `web/dist/index.html`
- [ ] `mise run release:production` compiles with the embedded frontend (`-tags spa`)
- [ ] Storybook builds without errors (`cd web && npx storybook build --quiet`)
- [ ] Dashboard story renders in Storybook with mocked data in both light and dark themes
- [ ] With the dev server running (`mise run dev:web` + Go daemon with `--dev`):
  - [ ] Dashboard loads and shows notifications from the API
  - [ ] WebSocket connects and status changes appear in real time
  - [ ] Pagination next/previous buttons work
  - [ ] Resend button appears on failed/not_sent rows
  - [ ] Clicking resend shows "Resending..." loading state, then the WebSocket pushes the `pending` state update and the row status changes
  - [ ] Error banner appears when API calls fail
  - [ ] Dark mode toggle switches themes correctly
  - [ ] Empty state message shows when no notifications exist
