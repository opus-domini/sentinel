import { useCallback, useEffect, useRef } from 'react'
import type {
  ApiFunction,
  InspectorSessionPatch,
  PresenceSocketRef,
  SeenCommandPayload,
} from './tmuxTypes'
import type { SessionActivityPatch } from '@/lib/tmuxSessionEvents'

type UseSeenTrackingOptions = {
  api: ApiFunction
  presenceSocketRef: PresenceSocketRef
  activeSession: string
  activeWindowIndex: number | null
  activePaneID: string | null
  applySessionActivityPatches: (
    rawPatches: Array<SessionActivityPatch> | undefined,
  ) => {
    hasInputPatches: boolean
    applied: boolean
    hasUnknownSession: boolean
  }
  applyInspectorProjectionPatches: (
    rawPatches: Array<InspectorSessionPatch> | undefined,
  ) => boolean
}

export function useSeenTracking(options: UseSeenTrackingOptions) {
  const {
    api,
    presenceSocketRef,
    activeSession,
    activeWindowIndex,
    activePaneID,
    applySessionActivityPatches,
    applyInspectorProjectionPatches,
  } = options

  const seenAckKeyRef = useRef('')
  const seenRequestSeqRef = useRef(0)
  const seenAckWaitersRef = useRef(new Map<string, (ok: boolean) => void>())

  const settlePendingSeenAcks = useCallback((ok: boolean) => {
    if (seenAckWaitersRef.current.size === 0) {
      return
    }
    const pending = Array.from(seenAckWaitersRef.current.values())
    seenAckWaitersRef.current.clear()
    for (const settle of pending) {
      settle(ok)
    }
  }, [])

  const sendSeenOverWS = useCallback(
    (payload: SeenCommandPayload) => {
      const socket = presenceSocketRef.current
      if (socket === null || socket.readyState !== WebSocket.OPEN) {
        return Promise.resolve(false)
      }

      seenRequestSeqRef.current += 1
      const requestId = `seen-${Date.now()}-${seenRequestSeqRef.current}`
      return new Promise<boolean>((resolve) => {
        let settled = false
        const settle = (ok: boolean) => {
          if (settled) return
          settled = true
          window.clearTimeout(timeoutID)
          seenAckWaitersRef.current.delete(requestId)
          resolve(ok)
        }
        const timeoutID = window.setTimeout(() => {
          settle(false)
        }, 800)
        seenAckWaitersRef.current.set(requestId, settle)
        try {
          socket.send(
            JSON.stringify({
              type: 'seen',
              requestId,
              ...payload,
            }),
          )
        } catch {
          settle(false)
        }
      })
    },
    [presenceSocketRef],
  )

  const markSeen = useCallback(
    async (params: {
      session: string
      scope: 'pane' | 'window' | 'session'
      paneId?: string
      windowIndex?: number
    }) => {
      const session = params.session.trim()
      if (session === '') return

      const body: {
        scope: 'pane' | 'window' | 'session'
        paneId?: string
        windowIndex?: number
      } = { scope: params.scope }
      if (params.scope === 'pane' && params.paneId) {
        body.paneId = params.paneId
      }
      if (params.scope === 'window' && Number.isInteger(params.windowIndex)) {
        body.windowIndex = params.windowIndex
      }

      try {
        if (
          await sendSeenOverWS({
            session,
            ...body,
          })
        ) {
          return
        }
      } catch {
        // Seen WS ack is best-effort.
      }

      try {
        const response = await api<{
          acked: boolean
          sessionPatches?: Array<SessionActivityPatch>
          inspectorPatches?: Array<InspectorSessionPatch>
        }>(`/api/tmux/sessions/${encodeURIComponent(session)}/seen`, {
          method: 'POST',
          body: JSON.stringify(body),
        })
        applySessionActivityPatches(response.sessionPatches)
        applyInspectorProjectionPatches(response.inspectorPatches)
      } catch {
        // Seen HTTP fallback is best-effort.
      }
    },
    [
      api,
      applyInspectorProjectionPatches,
      applySessionActivityPatches,
      sendSeenOverWS,
    ],
  )

  // Auto-mark seen when active selection changes
  useEffect(() => {
    const session = activeSession.trim()
    if (session === '') {
      seenAckKeyRef.current = ''
      return
    }

    if (activePaneID && activePaneID.trim() !== '') {
      const key = `${session}|pane|${activePaneID}`
      if (seenAckKeyRef.current !== key) {
        seenAckKeyRef.current = key
        void markSeen({ session, scope: 'pane', paneId: activePaneID })
      }
      return
    }

    if (activeWindowIndex !== null && activeWindowIndex >= 0) {
      const key = `${session}|window|${activeWindowIndex}`
      if (seenAckKeyRef.current !== key) {
        seenAckKeyRef.current = key
        void markSeen({
          session,
          scope: 'window',
          windowIndex: activeWindowIndex,
        })
      }
      return
    }

    const key = `${session}|session`
    if (seenAckKeyRef.current !== key) {
      seenAckKeyRef.current = key
      void markSeen({ session, scope: 'session' })
    }
  }, [activePaneID, activeWindowIndex, markSeen, activeSession])

  return {
    settlePendingSeenAcks,
    seenAckWaitersRef,
  }
}
