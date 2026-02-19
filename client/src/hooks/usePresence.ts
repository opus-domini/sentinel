import { useCallback, useEffect, useRef } from 'react'
import { resolvePresenceTerminalID } from './tmuxTypes'
import type { ApiFunction, PresenceSocketRef, TabsStateRef } from './tmuxTypes'
import { randomId } from '@/lib/utils'

type UsePresenceOptions = {
  api: ApiFunction
  presenceSocketRef: PresenceSocketRef
  tabsStateRef: TabsStateRef
  activeWindowIndex: number | null
  activePaneID: string | null
  activeSession: string
}

export function usePresence(options: UsePresenceOptions) {
  const {
    api,
    presenceSocketRef,
    tabsStateRef,
    activeWindowIndex,
    activePaneID,
    activeSession,
  } = options

  const presenceTerminalIDRef = useRef('')
  const presenceLastSignatureRef = useRef('')
  const presenceLastSentAtRef = useRef(0)
  const presenceHTTPInFlightRef = useRef(false)
  const activeWindowIndexRef = useRef<number | null>(null)
  const activePaneIDRef = useRef<string | null>(null)

  useEffect(() => {
    presenceTerminalIDRef.current = resolvePresenceTerminalID(randomId)
  }, [])

  useEffect(() => {
    activeWindowIndexRef.current = activeWindowIndex
    activePaneIDRef.current = activePaneID
  }, [activePaneID, activeWindowIndex])

  const buildPresencePayload = useCallback(() => {
    return {
      terminalId: presenceTerminalIDRef.current.trim(),
      session: tabsStateRef.current.activeSession.trim(),
      windowIndex: activeWindowIndexRef.current ?? -1,
      paneId: activePaneIDRef.current ?? '',
      visible: document.visibilityState === 'visible',
      focused: document.hasFocus(),
    }
  }, [tabsStateRef])

  const canEmitPresence = useCallback((signature: string, force: boolean) => {
    if (force) return true
    if (signature !== presenceLastSignatureRef.current) return true
    return Date.now() - presenceLastSentAtRef.current >= 10_000
  }, [])

  const markPresenceSent = useCallback((signature: string) => {
    presenceLastSignatureRef.current = signature
    presenceLastSentAtRef.current = Date.now()
  }, [])

  const sendPresenceOverWS = useCallback(
    (force = false): boolean => {
      const socket = presenceSocketRef.current
      if (socket === null || socket.readyState !== WebSocket.OPEN) {
        return false
      }

      const payload = buildPresencePayload()
      if (payload.terminalId === '') return false

      const signature = JSON.stringify(payload)
      if (!canEmitPresence(signature, force)) {
        return true
      }

      try {
        socket.send(
          JSON.stringify({
            type: 'presence',
            ...payload,
          }),
        )
        markPresenceSent(signature)
        return true
      } catch {
        return false
      }
    },
    [
      buildPresencePayload,
      canEmitPresence,
      markPresenceSent,
      presenceSocketRef,
    ],
  )

  const sendPresenceOverHTTP = useCallback(
    async (force = false) => {
      const payload = buildPresencePayload()
      if (payload.terminalId === '') return

      const signature = JSON.stringify(payload)
      if (!canEmitPresence(signature, force)) return
      if (presenceHTTPInFlightRef.current) return

      presenceHTTPInFlightRef.current = true
      try {
        await api<{ accepted: boolean }>('/api/tmux/presence', {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
        markPresenceSent(signature)
      } catch {
        // Presence fallback is best-effort.
      } finally {
        presenceHTTPInFlightRef.current = false
      }
    },
    [api, buildPresencePayload, canEmitPresence, markPresenceSent],
  )

  // Heartbeat interval
  useEffect(() => {
    const tick = () => {
      if (sendPresenceOverWS(false)) return
      void sendPresenceOverHTTP(false)
    }

    tick()
    const heartbeatID = window.setInterval(tick, 10_000)
    return () => {
      window.clearInterval(heartbeatID)
    }
  }, [sendPresenceOverHTTP, sendPresenceOverWS])

  // Visibility / focus / blur events
  useEffect(() => {
    const onPresenceSignal = () => {
      if (sendPresenceOverWS(true)) return
      void sendPresenceOverHTTP(true)
    }
    document.addEventListener('visibilitychange', onPresenceSignal)
    window.addEventListener('focus', onPresenceSignal)
    window.addEventListener('blur', onPresenceSignal)

    return () => {
      document.removeEventListener('visibilitychange', onPresenceSignal)
      window.removeEventListener('focus', onPresenceSignal)
      window.removeEventListener('blur', onPresenceSignal)
    }
  }, [sendPresenceOverHTTP, sendPresenceOverWS])

  // Re-emit when active pane/window/session changes
  useEffect(() => {
    if (sendPresenceOverWS(true)) return
    void sendPresenceOverHTTP(true)
  }, [
    activePaneID,
    activeWindowIndex,
    sendPresenceOverHTTP,
    sendPresenceOverWS,
    activeSession,
  ])

  return { sendPresenceOverWS }
}
