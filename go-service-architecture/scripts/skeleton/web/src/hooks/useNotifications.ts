import { useState, useEffect, useCallback, useRef } from 'react'
import type { Notification, NotificationStatus } from '../components'
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
  /** Navigate to a specific page (1-indexed). Only pages in the cursor stack are reachable. */
  goToPage: (page: number) => void
  /** Current page number (1-indexed). */
  currentPage: number
  /** Total number of pages. */
  totalPages: number
  /** Total number of notifications across all pages. */
  totalCount: number
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
  const [totalPages, setTotalPages] = useState(0)
  const [totalCount, setTotalCount] = useState(0)

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
      setTotalPages(data.meta.total_pages)
      setTotalCount(data.meta.total_count)
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

  const goToPage = useCallback(
    (page: number) => {
      if (page < 1 || page > totalPages) return
      // Pages are 1-indexed; cursor stack is 0-indexed.
      const targetIndex = page - 1
      if (targetIndex === pageIndex) return
      if (targetIndex < cursorStack.length) {
        setPageIndex(targetIndex)
        fetchPage(cursorStack[targetIndex])
      }
      // Pages beyond the cursor stack are not directly reachable
      // (positional context only -- resolved ambiguity #2).
    },
    [totalPages, pageIndex, cursorStack, fetchPage],
  )

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
    goToPage,
    currentPage: pageIndex + 1,
    totalPages,
    totalCount,
    resend,
    resending,
  }
}
