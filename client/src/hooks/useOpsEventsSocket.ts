import { useEffect, useRef, useState } from 'react'
import type { ConnectionState } from '@/types'
import { buildWSProtocols } from '@/lib/wsAuth'
import { createReconnect } from '@/lib/wsReconnect'

type UseOpsEventsSocketOptions = {
  token: string
  tokenRequired: boolean
  onMessage: (message: unknown) => void
}

export function useOpsEventsSocket({
  token,
  tokenRequired,
  onMessage,
}: UseOpsEventsSocketOptions): ConnectionState {
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('connecting')

  const onMessageRef = useRef(onMessage)
  useEffect(() => {
    onMessageRef.current = onMessage
  }, [onMessage])

  useEffect(() => {
    if (tokenRequired && token.trim() === '') {
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

      const wsURL = new URL('/ws/events', window.location.origin)
      wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:'

      socket = new WebSocket(wsURL.toString(), buildWSProtocols(token))

      socket.onopen = () => {
        if (disposed) return
        reconnect.reset()
        setConnectionState('connected')
      }

      socket.onmessage = (event) => {
        if (disposed) return
        let message: unknown
        try {
          message = JSON.parse(String(event.data))
        } catch {
          return
        }
        if (typeof message !== 'object' || message === null) return
        onMessageRef.current(message)
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
  }, [token, tokenRequired])

  return connectionState
}
