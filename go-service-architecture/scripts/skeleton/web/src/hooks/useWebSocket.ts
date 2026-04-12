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
