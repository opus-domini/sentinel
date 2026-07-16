import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ConnectionState } from '@/types'
import { buildWSProtocols } from '@/lib/wsAuth'
import { createReconnect } from '@/lib/wsReconnect'

type Subscriber = (message: unknown) => void

/**
 * Manages a single shared WebSocket connection to /ws/events for ops routes.
 * The socket activates when at least one subscriber is registered and
 * disconnects when all subscribers are removed.  This prevents Chrome's
 * per-origin HTTP/1.1 socket pool (max 6) from being exhausted by
 * independent connections on every route transition.
 */
export function useSharedOpsEventsSocket(options: {
  authenticated: boolean
  tokenRequired: boolean
  connectionReady?: boolean
}) {
  const { authenticated, tokenRequired, connectionReady = true } = options
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected')
  const subscribersRef = useRef(new Set<Subscriber>())
  const socketRef = useRef<WebSocket | null>(null)
  const retryTimerRef = useRef<number | null>(null)
  const reconnectRef = useRef(createReconnect())
  const disposedRef = useRef(false)

  const clearRetry = useCallback(() => {
    if (retryTimerRef.current != null) {
      window.clearTimeout(retryTimerRef.current)
      retryTimerRef.current = null
    }
  }, [])

  const disconnect = useCallback(() => {
    clearRetry()
    const sock = socketRef.current
    socketRef.current = null
    if (sock != null) {
      try {
        sock.close()
      } catch {
        // ignore
      }
    }
    setConnectionState('disconnected')
  }, [clearRetry])

  const connect = useCallback(() => {
    if (disposedRef.current) return
    if (!connectionReady) return
    if (tokenRequired && !authenticated) return
    if (subscribersRef.current.size === 0) return
    if (document.visibilityState === 'hidden') return
    clearRetry()
    setConnectionState('connecting')

    const wsURL = new URL('/ws/events', window.location.origin)
    wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:'

    const socket = new WebSocket(wsURL.toString(), buildWSProtocols())
    socketRef.current = socket

    socket.onopen = () => {
      if (socketRef.current !== socket) return
      reconnectRef.current.reset()
      setConnectionState('connected')
    }

    socket.onmessage = (event) => {
      if (socketRef.current !== socket) return
      let message: unknown
      try {
        message = JSON.parse(String(event.data))
      } catch {
        return
      }
      if (typeof message !== 'object' || message === null) return
      for (const handler of subscribersRef.current) {
        try {
          handler(message)
        } catch {
          // keep stream alive
        }
      }
    }

    socket.onerror = () => {
      if (socketRef.current !== socket) return
      setConnectionState('error')
    }

    socket.onclose = () => {
      if (socketRef.current !== socket) return
      socketRef.current = null
      setConnectionState('disconnected')
      if (disposedRef.current || subscribersRef.current.size === 0) return
      clearRetry()
      retryTimerRef.current = window.setTimeout(connect, reconnectRef.current.next())
    }
  }, [authenticated, clearRetry, connectionReady, tokenRequired])

  const reconnectNow = useCallback(() => {
    if (
      disposedRef.current ||
      !connectionReady ||
      subscribersRef.current.size === 0 ||
      (tokenRequired && !authenticated) ||
      document.visibilityState === 'hidden'
    ) {
      return
    }
    clearRetry()
    reconnectRef.current.reset()
    const sock = socketRef.current
    socketRef.current = null
    if (sock != null) {
      try {
        sock.close()
      } catch {
        // ignore
      }
    }
    connect()
  }, [authenticated, clearRetry, connect, connectionReady, tokenRequired])

  useEffect(() => {
    const handleVisible = () => {
      if (document.visibilityState === 'visible') reconnectNow()
    }
    const handleOnline = () => reconnectNow()
    document.addEventListener('visibilitychange', handleVisible)
    window.addEventListener('online', handleOnline)
    return () => {
      document.removeEventListener('visibilitychange', handleVisible)
      window.removeEventListener('online', handleOnline)
    }
  }, [reconnectNow])

  // Reconnect when auth changes while subscribers exist
  useEffect(() => {
    if (subscribersRef.current.size > 0 && socketRef.current === null) {
      connect()
    }
  }, [authenticated, connectionReady, tokenRequired, connect])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disposedRef.current = true
      disconnect()
    }
  }, [disconnect])

  const subscribe = useCallback(
    (handler: Subscriber) => {
      subscribersRef.current.add(handler)
      // Activate connection on first subscriber.
      if (subscribersRef.current.size === 1 && socketRef.current === null) {
        connect()
      }
      return () => {
        subscribersRef.current.delete(handler)
        // Disconnect when last subscriber leaves
        if (subscribersRef.current.size === 0) {
          disconnect()
        }
      }
    },
    [connect, disconnect],
  )

  const forceReconnect = useCallback(() => {
    clearRetry()
    reconnectRef.current.reset()

    const sock = socketRef.current
    socketRef.current = null
    if (sock != null) {
      try {
        sock.close()
      } catch {
        // ignore
      }
    }

    if (
      disposedRef.current ||
      !connectionReady ||
      subscribersRef.current.size === 0 ||
      (tokenRequired && !authenticated) ||
      document.visibilityState === 'hidden'
    ) {
      setConnectionState('disconnected')
      return
    }

    connect()
  }, [authenticated, clearRetry, connect, connectionReady, tokenRequired])

  return useMemo(
    () => ({ connectionState, forceReconnect, subscribe }),
    [connectionState, forceReconnect, subscribe],
  )
}
