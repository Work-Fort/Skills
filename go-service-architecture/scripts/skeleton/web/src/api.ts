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
