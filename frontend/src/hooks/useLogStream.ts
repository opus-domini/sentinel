import { useEffect, useRef, useState } from 'react'

import type { ConnectionState } from '@/types'
import { buildWSProtocols } from '@/lib/wsAuth'
import { createReconnect } from '@/lib/wsReconnect'

type LogStreamTarget =
  | { kind: 'service'; name: string }
  | { kind: 'unit'; unit: string; scope: string; manager: string }

type UseLogStreamOptions = {
  authenticated: boolean
  tokenRequired: boolean
  target: LogStreamTarget | null
  enabled: boolean
  onLine: (line: string) => void
}

export function useLogStream({
  authenticated,
  tokenRequired,
  target,
  enabled,
  onLine,
}: UseLogStreamOptions): ConnectionState {
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('disconnected')

  const onLineRef = useRef(onLine)
  useEffect(() => {
    onLineRef.current = onLine
  }, [onLine])

  useEffect(() => {
    if (!enabled || target == null) {
      setConnectionState('disconnected')
      return
    }
    if (tokenRequired && !authenticated) {
      setConnectionState('disconnected')
      return
    }

    let disposed = false
    let socket: WebSocket | null = null
    let retryTimer: number | null = null
    const reconnect = createReconnect()

    const clearRetry = () => {
      if (retryTimer != null) {
        window.clearTimeout(retryTimer)
        retryTimer = null
      }
    }

    const connect = () => {
      if (disposed) return
      clearRetry()
      setConnectionState('connecting')

      const params = new URLSearchParams()
      if (target.kind === 'service') {
        params.set('service', target.name)
      } else {
        params.set('unit', target.unit)
        params.set('scope', target.scope)
        params.set('manager', target.manager)
      }

      const wsURL = new URL(
        `/ws/logs?${params.toString()}`,
        window.location.origin,
      )
      wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:'

      socket = new WebSocket(wsURL.toString(), buildWSProtocols())

      socket.onopen = () => {
        if (disposed) return
        reconnect.reset()
        setConnectionState('connected')
      }

      socket.onmessage = (event) => {
        if (disposed) return
        let msg: unknown
        try {
          msg = JSON.parse(String(event.data))
        } catch {
          return
        }
        if (typeof msg !== 'object' || msg === null) return
        const typed = msg as { type?: string; line?: string }
        if (typed.type === 'log' && typeof typed.line === 'string') {
          onLineRef.current(typed.line)
        }
      }

      socket.onerror = () => {
        if (!disposed) {
          setConnectionState('error')
        }
      }

      socket.onclose = () => {
        if (disposed) return
        setConnectionState('disconnected')
        clearRetry()
        retryTimer = window.setTimeout(connect, reconnect.next())
      }
    }

    connect()
    return () => {
      disposed = true
      clearRetry()
      if (socket != null) {
        try {
          socket.close()
        } catch {
          // ignore close race
        }
      }
    }
  }, [authenticated, tokenRequired, target, enabled])

  return connectionState
}
