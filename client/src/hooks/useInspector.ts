import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  asNonNegativeInt,
  asNonNegativeInt64,
  isTmuxBinaryMissingMessage,
  samePaneProjection,
  sameWindowProjection,
} from './tmuxTypes'
import { GuardrailConfirmError } from './useTmuxApi'
import type {
  ConnectionState,
  PaneInfo,
  PanesResponse,
  Session,
  WindowInfo,
  WindowsResponse,
} from '@/types'
import type { TmuxInspectorSnapshot } from '@/lib/tmuxQueryCache'
import type {
  SessionActivityPatch,
  SessionPatchApplyResult,
} from '@/lib/tmuxSessionEvents'
import type {
  ApiFunction,
  InspectorSessionPatch,
  RuntimeMetrics,
  TabsStateRef,
} from './tmuxTypes'
import {
  shouldCacheActiveInspectorSnapshot,
  tmuxInspectorQueryKey,
} from '@/lib/tmuxQueryCache'
import { shouldSkipInspectorRefresh } from '@/lib/tmuxInspectorRefresh'
import { moveSidebarItem } from '@/lib/sessionSidebarOrder'
import { sanitizeTmuxPaneTitle, sanitizeTmuxWindowName } from '@/lib/tmuxName'
import {
  addPendingPaneClose,
  addPendingWindowClose,
  addPendingWindowCreate,
  buildPendingSplitPaneID,
  clearPendingPaneClosesForSession,
  clearPendingWindowClosesForSession,
  clearPendingWindowCreatesForSession,
  clearPendingWindowPaneFloor,
  clearPendingWindowPaneFloorsForSession,
  isPendingSplitPaneID,
  mergePendingInspectorSnapshot,
  removePendingPaneClose,
  removePendingWindowClose,
  removePendingWindowCreate,
  setPendingWindowPaneFloor,
} from '@/lib/tmuxInspectorOptimistic'

function stabilizeWindows(
  previousWindows: Array<WindowInfo>,
  nextWindows: Array<WindowInfo>,
): Array<WindowInfo> {
  if (previousWindows.length === 0 || nextWindows.length === 0) {
    return nextWindows
  }

  const previousByRuntime = new Map<string, WindowInfo>()
  const previousByManaged = new Map<string, WindowInfo>()
  const previousByIndex = new Map<string, WindowInfo>()

  for (const previous of previousWindows) {
    const runtimeID = (previous.tmuxWindowId ?? '').trim()
    if (runtimeID !== '') {
      previousByRuntime.set(runtimeID, previous)
    }

    const managedWindowID = (previous.managedWindowId ?? '').trim()
    if (managedWindowID !== '') {
      previousByManaged.set(managedWindowID, previous)
    }

    previousByIndex.set(`${previous.session}:${previous.index}`, previous)
  }

  return nextWindows.map((next) => {
    const runtimeID = (next.tmuxWindowId ?? '').trim()
    const managedWindowID = (next.managedWindowId ?? '').trim()
    let previous: WindowInfo | undefined
    if (runtimeID !== '') {
      previous = previousByRuntime.get(runtimeID)
    } else if (managedWindowID !== '') {
      previous = previousByManaged.get(managedWindowID)
    } else {
      previous = previousByIndex.get(`${next.session}:${next.index}`)
    }

    if (!previous) {
      return next
    }

    const merged = { ...next }
    if (merged.name.trim() === '' && previous.name.trim() !== '') {
      merged.name = previous.name
    }
    if (
      merged.displayName.trim() === '' &&
      previous.displayName.trim() !== ''
    ) {
      merged.displayName = previous.displayName
    }
    if ((merged.tmuxWindowId ?? '').trim() === '') {
      merged.tmuxWindowId = previous.tmuxWindowId
    }

    const sameRuntimeIdentity =
      runtimeID !== '' && runtimeID === (previous.tmuxWindowId ?? '').trim()
    const sameManagedIdentity =
      managedWindowID !== '' &&
      managedWindowID === (previous.managedWindowId ?? '').trim()

    if (!sameRuntimeIdentity && !sameManagedIdentity) {
      return merged
    }

    if (previous.managed) {
      merged.managed = true
      if ((merged.managedWindowId ?? '').trim() === '') {
        merged.managedWindowId = previous.managedWindowId
      }
      if ((merged.launcherId ?? '').trim() === '') {
        merged.launcherId = previous.launcherId
      }
      if ((merged.displayIcon ?? '').trim() === '') {
        merged.displayIcon = previous.displayIcon
      }
      const displayNameLooksDegraded =
        merged.displayName.trim() === '' ||
        merged.displayName.trim() === merged.name.trim()
      if (displayNameLooksDegraded && previous.displayName.trim() !== '') {
        merged.displayName = previous.displayName
      }
    }

    return merged
  })
}

type UseInspectorOptions = {
  api: ApiFunction
  tabsStateRef: TabsStateRef
  sessionsRef: React.MutableRefObject<Array<Session>>
  runtimeMetricsRef: React.MutableRefObject<RuntimeMetrics>
  activeSession: string
  setTmuxUnavailable: (unavailable: boolean) => void
  setSessions: React.Dispatch<React.SetStateAction<Array<Session>>>
  refreshSessions: () => Promise<void>
  pushErrorToast: (title: string, message: string) => void
  pushSuccessToast: (title: string, message: string) => void
  setConnection: (state: ConnectionState, detail: string) => void
  requestGuardrailConfirm: (
    ruleName: string,
    message: string,
    onConfirm: () => void,
  ) => void
}

export function useInspector(options: UseInspectorOptions) {
  const {
    api,
    tabsStateRef,
    sessionsRef,
    runtimeMetricsRef,
    activeSession,
    setTmuxUnavailable,
    setSessions,
    refreshSessions,
    pushErrorToast,
    pushSuccessToast,
    setConnection,
    requestGuardrailConfirm,
  } = options

  const queryClient = useQueryClient()
  const inspectorGenerationRef = useRef(0)
  const pendingCreateSessionsRef = useRef(new Map<string, string>())
  const pendingCreateWindowsRef = useRef(new Map<string, Set<number>>())
  const pendingCloseWindowsRef = useRef(new Map<string, Set<number>>())
  const pendingClosePanesRef = useRef(new Map<string, Set<string>>())
  const pendingWindowPaneFloorsRef = useRef(
    new Map<string, Map<number, number>>(),
  )
  const pendingSelectMutationKindRef = useRef<'window' | 'pane' | null>(null)

  const [windows, setWindows] = useState<Array<WindowInfo>>(() => {
    const active = activeSession.trim()
    if (active === '') return []
    return (
      queryClient.getQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(active),
      )?.windows ?? []
    )
  })
  const [panes, setPanes] = useState<Array<PaneInfo>>(() => {
    const active = activeSession.trim()
    if (active === '') return []
    return (
      queryClient.getQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(active),
      )?.panes ?? []
    )
  })
  const [activeWindowIndexOverride, setActiveWindowIndexOverride] = useState<
    number | null
  >(null)
  const [activePaneIDOverride, setActivePaneIDOverride] = useState<
    string | null
  >(null)
  const [inspectorLoading, setInspectorLoading] = useState(false)
  const [inspectorError, setInspectorError] = useState('')
  const [renameWindowDialogOpen, setRenameWindowDialogOpen] = useState(false)
  const [renameWindowTarget, setRenameWindowTarget] = useState<{
    session: string
    index: number
  } | null>(null)
  const [renameWindowValue, setRenameWindowValue] = useState('')
  const [renamePaneDialogOpen, setRenamePaneDialogOpen] = useState(false)
  const [renamePaneTarget, setRenamePaneTarget] = useState<{
    session: string
    paneID: string
  } | null>(null)
  const [renamePaneValue, setRenamePaneValue] = useState('')

  const windowsRef = useRef<Array<WindowInfo>>([])
  const panesRef = useRef<Array<PaneInfo>>([])
  const inspectorLoadingRef = useRef(false)
  const activeWindowOverrideRef = useRef<number | null>(null)
  const activePaneOverrideRef = useRef<string | null>(null)
  // Tracks in-flight selectWindow / selectPane API calls. While > 0, the
  // server state may be stale relative to the optimistic override.
  const selectInFlightRef = useRef(0)

  // Sync refs
  useEffect(() => {
    windowsRef.current = windows
  }, [windows])
  useEffect(() => {
    panesRef.current = panes
  }, [panes])
  useEffect(() => {
    inspectorLoadingRef.current = inspectorLoading
  }, [inspectorLoading])
  useEffect(() => {
    activeWindowOverrideRef.current = activeWindowIndexOverride
  }, [activeWindowIndexOverride])
  useEffect(() => {
    activePaneOverrideRef.current = activePaneIDOverride
  }, [activePaneIDOverride])

  // Cache inspector snapshot
  useEffect(() => {
    const active = activeSession.trim()
    if (!shouldCacheActiveInspectorSnapshot(active, windows, panes)) {
      return
    }
    queryClient.setQueryData<TmuxInspectorSnapshot>(
      tmuxInspectorQueryKey(active),
      {
        windows,
        panes,
      },
    )
  }, [panes, queryClient, activeSession, windows])

  // Restore from cache on session switch
  useEffect(() => {
    const active = activeSession.trim()
    setRenameWindowDialogOpen(false)
    setRenameWindowTarget(null)
    setRenameWindowValue('')
    setRenamePaneDialogOpen(false)
    setRenamePaneTarget(null)
    setRenamePaneValue('')
    if (active === '') {
      setActiveWindowIndexOverride(null)
      setActivePaneIDOverride(null)
      return
    }
    const cached = queryClient.getQueryData<TmuxInspectorSnapshot>(
      tmuxInspectorQueryKey(active),
    )
    if (cached) {
      setWindows((prev) =>
        sameWindowProjection(prev, cached.windows) ? prev : cached.windows,
      )
      setPanes((prev) =>
        samePaneProjection(prev, cached.panes) ? prev : cached.panes,
      )
      setInspectorError('')
      setInspectorLoading(false)
      inspectorLoadingRef.current = false
    } else {
      setWindows([])
      setPanes([])
    }
    setActiveWindowIndexOverride(null)
    setActivePaneIDOverride(null)
  }, [queryClient, activeSession])

  const clearPendingInspectorSessionState = useCallback((session: string) => {
    clearPendingWindowCreatesForSession(
      pendingCreateWindowsRef.current,
      session,
    )
    clearPendingWindowClosesForSession(pendingCloseWindowsRef.current, session)
    clearPendingPaneClosesForSession(pendingClosePanesRef.current, session)
    clearPendingWindowPaneFloorsForSession(
      pendingWindowPaneFloorsRef.current,
      session,
    )
  }, [])

  const mergeInspectorSnapshotWithPending = useCallback(
    (
      session: string,
      sourceWindows: Array<WindowInfo>,
      sourcePanes: Array<PaneInfo>,
    ) => {
      const merged = mergePendingInspectorSnapshot(
        session,
        sourceWindows,
        sourcePanes,
        {
          pendingWindowCreates: pendingCreateWindowsRef.current,
          pendingWindowCloses: pendingCloseWindowsRef.current,
          pendingPaneCloses: pendingClosePanesRef.current,
          pendingWindowPaneFloors: pendingWindowPaneFloorsRef.current,
          optimisticVisibleWindowBaseline: Math.max(
            0,
            windowsRef.current.filter(
              (windowInfo) => windowInfo.session === session,
            ).length -
              (pendingCreateWindowsRef.current.get(session)?.size ?? 0),
          ),
        },
      )
      for (const index of merged.confirmedWindowCreates) {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          session,
          index,
        )
      }
      for (const index of merged.confirmedWindowCloses) {
        removePendingWindowClose(pendingCloseWindowsRef.current, session, index)
      }
      for (const paneID of merged.confirmedPaneCloses) {
        removePendingPaneClose(pendingClosePanesRef.current, session, paneID)
      }
      for (const index of merged.confirmedWindowPaneFloors) {
        clearPendingWindowPaneFloor(
          pendingWindowPaneFloorsRef.current,
          session,
          index,
        )
      }
      return merged
    },
    [],
  )

  const applySessionActivityPatches = useCallback(
    (
      rawPatches: Array<SessionActivityPatch> | undefined,
    ): SessionPatchApplyResult => {
      if (!Array.isArray(rawPatches) || rawPatches.length === 0) {
        return {
          hasInputPatches: false,
          applied: false,
          hasUnknownSession: false,
        }
      }

      const knownSessions = new Set(
        sessionsRef.current.map((item) => item.name.trim()),
      )
      const trackedSessions = new Set<string>()
      const activeSessionName = tabsStateRef.current.activeSession.trim()
      if (activeSessionName !== '') {
        trackedSessions.add(activeSessionName)
      }
      for (const tab of tabsStateRef.current.openTabs) {
        const name = tab.trim()
        if (name !== '') {
          trackedSessions.add(name)
        }
      }

      let hasInputPatches = false
      let hasUnknownSession = false
      const patchesByName = new Map<string, SessionActivityPatch>()
      for (const patch of rawPatches) {
        const name = patch.name?.trim() ?? ''
        if (name === '') continue
        hasInputPatches = true
        if (!trackedSessions.has(name)) {
          continue
        }
        if (!knownSessions.has(name)) {
          hasUnknownSession = true
          continue
        }
        patchesByName.set(name, patch)
      }
      if (patchesByName.size === 0) {
        return { hasInputPatches, applied: false, hasUnknownSession }
      }

      setSessions((prev) =>
        prev.map((item) => {
          const patch = patchesByName.get(item.name)
          if (!patch) return item

          const activityAt =
            typeof patch.activityAt === 'string' &&
            patch.activityAt.trim() !== ''
              ? patch.activityAt
              : item.activityAt
          const lastContent =
            typeof patch.lastContent === 'string'
              ? patch.lastContent
              : item.lastContent
          const next = {
            ...item,
            attached: asNonNegativeInt(patch.attached, item.attached),
            windows: asNonNegativeInt(patch.windows, item.windows),
            panes: asNonNegativeInt(patch.panes, item.panes),
            activityAt,
            lastContent,
            unreadWindows: asNonNegativeInt(
              patch.unreadWindows,
              item.unreadWindows ?? 0,
            ),
            unreadPanes: asNonNegativeInt(
              patch.unreadPanes,
              item.unreadPanes ?? 0,
            ),
            rev: asNonNegativeInt64(patch.rev, item.rev ?? 0),
          }
          if (
            next.attached === item.attached &&
            next.windows === item.windows &&
            next.panes === item.panes &&
            next.activityAt === item.activityAt &&
            next.lastContent === item.lastContent &&
            next.unreadWindows === (item.unreadWindows ?? 0) &&
            next.unreadPanes === (item.unreadPanes ?? 0) &&
            next.rev === (item.rev ?? 0)
          ) {
            return item
          }
          return next
        }),
      )

      return { hasInputPatches, applied: true, hasUnknownSession }
    },
    [sessionsRef, setSessions, tabsStateRef],
  )

  const applyInspectorProjectionPatches = useCallback(
    (rawPatches: Array<InspectorSessionPatch> | undefined): boolean => {
      if (!Array.isArray(rawPatches) || rawPatches.length === 0) {
        return false
      }

      const activeSessionName = tabsStateRef.current.activeSession.trim()
      if (activeSessionName === '') {
        return false
      }

      const targetPatch = rawPatches.find(
        (patch) => (patch.session?.trim() ?? '') === activeSessionName,
      )
      if (!targetPatch) {
        return false
      }

      let nextWindows: Array<WindowInfo> | null = null
      if (Array.isArray(targetPatch.windows)) {
        const parsedWindows: Array<WindowInfo> = []
        for (const rawWindow of targetPatch.windows) {
          const session = (rawWindow.session?.trim() ?? '') || activeSessionName
          if (session !== activeSessionName) continue
          const index = rawWindow.index
          if (
            typeof index !== 'number' ||
            !Number.isFinite(index) ||
            index < 0
          ) {
            continue
          }

          const unreadPanes = asNonNegativeInt(rawWindow.unreadPanes, 0)
          parsedWindows.push({
            session,
            index: Math.trunc(index),
            name: rawWindow.name ?? '',
            displayName:
              typeof rawWindow.displayName === 'string' &&
              rawWindow.displayName.trim() !== ''
                ? rawWindow.displayName
                : (rawWindow.name ?? ''),
            displayIcon:
              typeof rawWindow.displayIcon === 'string'
                ? rawWindow.displayIcon
                : undefined,
            tmuxWindowId:
              typeof rawWindow.tmuxWindowId === 'string'
                ? rawWindow.tmuxWindowId
                : undefined,
            managed: rawWindow.managed === true,
            managedWindowId:
              typeof rawWindow.managedWindowId === 'string'
                ? rawWindow.managedWindowId
                : undefined,
            launcherId:
              typeof rawWindow.launcherId === 'string'
                ? rawWindow.launcherId
                : undefined,
            active: rawWindow.active === true,
            panes: asNonNegativeInt(rawWindow.panes, 0),
            unreadPanes,
            hasUnread:
              typeof rawWindow.hasUnread === 'boolean'
                ? rawWindow.hasUnread
                : unreadPanes > 0,
            rev: asNonNegativeInt64(rawWindow.rev, 0),
            activityAt:
              typeof rawWindow.activityAt === 'string'
                ? rawWindow.activityAt
                : undefined,
          })
        }
        parsedWindows.sort((left, right) => left.index - right.index)
        nextWindows = stabilizeWindows(windowsRef.current, parsedWindows)
      }

      let nextPanes: Array<PaneInfo> | null = null
      if (Array.isArray(targetPatch.panes)) {
        const parsedPanes: Array<PaneInfo> = []
        for (const rawPane of targetPatch.panes) {
          const session = (rawPane.session?.trim() ?? '') || activeSessionName
          if (session !== activeSessionName) continue
          const windowIndex = rawPane.windowIndex
          const paneIndex = rawPane.paneIndex
          const paneId = rawPane.paneId?.trim() ?? ''
          if (
            typeof windowIndex !== 'number' ||
            !Number.isFinite(windowIndex) ||
            windowIndex < 0 ||
            typeof paneIndex !== 'number' ||
            !Number.isFinite(paneIndex) ||
            paneIndex < 0 ||
            paneId === ''
          ) {
            continue
          }

          const revision = asNonNegativeInt64(rawPane.revision, 0)
          const seenRevision = asNonNegativeInt64(rawPane.seenRevision, 0)
          parsedPanes.push({
            session,
            windowIndex: Math.trunc(windowIndex),
            paneIndex: Math.trunc(paneIndex),
            paneId,
            title: rawPane.title ?? '',
            active: rawPane.active === true,
            tty: rawPane.tty ?? '',
            tailPreview:
              typeof rawPane.tailPreview === 'string'
                ? rawPane.tailPreview
                : undefined,
            revision,
            seenRevision,
            hasUnread:
              typeof rawPane.hasUnread === 'boolean'
                ? rawPane.hasUnread
                : revision > seenRevision,
            changedAt:
              typeof rawPane.changedAt === 'string'
                ? rawPane.changedAt
                : undefined,
          })
        }
        parsedPanes.sort((left, right) => {
          if (left.windowIndex !== right.windowIndex) {
            return left.windowIndex - right.windowIndex
          }
          return left.paneIndex - right.paneIndex
        })
        nextPanes = parsedPanes
      }

      if (nextWindows === null && nextPanes === null) {
        return false
      }

      const merged = mergeInspectorSnapshotWithPending(
        activeSessionName,
        nextWindows ?? windowsRef.current,
        nextPanes ?? panesRef.current,
      )
      const stableWindows = stabilizeWindows(windowsRef.current, merged.windows)
      queryClient.setQueryData<TmuxInspectorSnapshot>(
        tmuxInspectorQueryKey(activeSessionName),
        {
          windows: stableWindows,
          panes: merged.panes,
        },
      )

      setWindows((prev) =>
        sameWindowProjection(prev, stableWindows) ? prev : stableWindows,
      )
      setPanes((prev) =>
        samePaneProjection(prev, merged.panes) ? prev : merged.panes,
      )

      const windowOverride = activeWindowOverrideRef.current
      if (
        windowOverride !== null &&
        !stableWindows.some((windowInfo) => windowInfo.index === windowOverride)
      ) {
        setActiveWindowIndexOverride(null)
      }

      const paneOverride = activePaneOverrideRef.current
      if (
        paneOverride !== null &&
        !merged.panes.some((paneInfo) => paneInfo.paneId === paneOverride)
      ) {
        setActivePaneIDOverride(null)
      }

      return true
    },
    [mergeInspectorSnapshotWithPending, queryClient, tabsStateRef],
  )

  const refreshInspector = useCallback(
    async (target: string, params?: { background?: boolean }) => {
      runtimeMetricsRef.current.inspectorRefreshCount += 1
      const session = target.trim()
      const bg = params?.background === true
      if (session === '') {
        setWindows([])
        setPanes([])
        setActiveWindowIndexOverride(null)
        setActivePaneIDOverride(null)
        setInspectorError('')
        setInspectorLoading(false)
        inspectorLoadingRef.current = false
        return
      }
      if (shouldSkipInspectorRefresh(bg, inspectorLoadingRef.current)) {
        return
      }
      const gen = ++inspectorGenerationRef.current
      if (!bg) {
        inspectorLoadingRef.current = true
        setInspectorLoading(true)
      }
      setInspectorError('')
      try {
        const windowsResponse = await api<WindowsResponse>(
          `/api/tmux/sessions/${encodeURIComponent(session)}/windows`,
        )
        if (gen !== inspectorGenerationRef.current) return
        const panesResponse = await api<PanesResponse>(
          `/api/tmux/sessions/${encodeURIComponent(session)}/panes`,
        )
        if (gen !== inspectorGenerationRef.current) return
        const merged = mergeInspectorSnapshotWithPending(
          session,
          windowsResponse.windows,
          panesResponse.panes,
        )
        const stableWindows = stabilizeWindows(
          windowsRef.current,
          merged.windows,
        )
        queryClient.setQueryData<TmuxInspectorSnapshot>(
          tmuxInspectorQueryKey(session),
          {
            windows: stableWindows,
            panes: merged.panes,
          },
        )
        setWindows((prev) =>
          sameWindowProjection(prev, stableWindows) ? prev : stableWindows,
        )
        setPanes((prev) =>
          samePaneProjection(prev, merged.panes) ? prev : merged.panes,
        )

        const windowOverride = activeWindowOverrideRef.current
        const paneOverride = activePaneOverrideRef.current
        const fetchedActiveWindow =
          stableWindows.find((windowInfo) => windowInfo.active)?.index ?? null
        const fetchedActivePane =
          merged.panes.find((paneInfo) => paneInfo.active)?.paneId ?? null

        const keepWindowOverride =
          windowOverride !== null &&
          selectInFlightRef.current > 0 &&
          fetchedActiveWindow !== windowOverride &&
          stableWindows.some(
            (windowInfo) => windowInfo.index === windowOverride,
          )
        const keepPaneOverride =
          pendingSelectMutationKindRef.current === 'pane' &&
          paneOverride !== null &&
          selectInFlightRef.current > 0 &&
          fetchedActivePane !== paneOverride &&
          merged.panes.some((paneInfo) => paneInfo.paneId === paneOverride)

        if (!keepWindowOverride) {
          setActiveWindowIndexOverride(null)
        }
        if (!keepPaneOverride) {
          setActivePaneIDOverride(null)
        }
        if (!keepWindowOverride && !keepPaneOverride) {
          pendingSelectMutationKindRef.current = null
          if (selectInFlightRef.current > 0) {
            selectInFlightRef.current = 0
          }
        }
      } catch (error) {
        if (gen !== inspectorGenerationRef.current) return
        if (pendingCreateSessionsRef.current.has(session)) {
          return
        }
        const message =
          error instanceof Error
            ? error.message
            : 'failed to load session details'
        const unavailable = isTmuxBinaryMissingMessage(message)
        if (unavailable) {
          setTmuxUnavailable(true)
        }
        setInspectorError(message)
      } finally {
        if (gen === inspectorGenerationRef.current && !bg) {
          inspectorLoadingRef.current = false
          setInspectorLoading(false)
        }
      }
    },
    [
      api,
      mergeInspectorSnapshotWithPending,
      queryClient,
      runtimeMetricsRef,
      setTmuxUnavailable,
    ],
  )

  // Keep inspector in sync when user switches active session/tab
  useEffect(() => {
    void refreshInspector(activeSession)
  }, [refreshInspector, activeSession])

  const reorderWindows = useCallback(
    (activeWindowID: string, overWindowID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return

      const orderedWindows = [...windowsRef.current].sort(
        (left, right) => left.index - right.index,
      )
      if (orderedWindows.length < 2) {
        return
      }

      const currentOrder = orderedWindows.map((windowInfo) =>
        (windowInfo.tmuxWindowId ?? '').trim(),
      )
      if (currentOrder.some((windowID) => windowID === '')) {
        pushErrorToast(
          'Reorder Windows',
          'window order is not ready yet; refresh and try again',
        )
        return
      }

      const nextOrder = moveSidebarItem(
        currentOrder,
        activeWindowID,
        overWindowID,
      )
      if (nextOrder === currentOrder) {
        return
      }

      setInspectorError('')
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/windows/order`,
        {
          method: 'PATCH',
          body: JSON.stringify({ windowIds: nextOrder }),
        },
      )
        .then(() => {
          void refreshInspector(active, { background: true })
        })
        .catch((error) => {
          const msg =
            error instanceof Error ? error.message : 'failed to reorder windows'
          setInspectorError(msg)
          pushErrorToast('Reorder Windows', msg)
          void refreshInspector(active, { background: true })
        })
    },
    [api, pushErrorToast, refreshInspector, tabsStateRef],
  )

  const selectWindow = useCallback(
    (windowIndex: number) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      if (activeWindowOverrideRef.current === windowIndex) return
      setInspectorError('')
      // Bump generation to invalidate any in-flight refreshInspector that
      // might carry stale data from before the user's click.
      inspectorGenerationRef.current += 1
      // If a foreground refresh was in-flight, its finally block will skip
      // the loading clear (gen mismatch).  Reset loading here to prevent
      // the WindowStrip from being stuck on skeleton loaders.
      if (inspectorLoadingRef.current) {
        inspectorLoadingRef.current = false
        setInspectorLoading(false)
      }
      pendingSelectMutationKindRef.current = 'window'
      selectInFlightRef.current += 1
      setActiveWindowIndexOverride(windowIndex)
      const preferredPaneID =
        panes.find((p) => p.windowIndex === windowIndex && p.active)?.paneId ??
        panes.find((p) => p.windowIndex === windowIndex)?.paneId ??
        null
      setActivePaneIDOverride(preferredPaneID)
      setWindows((prev) =>
        prev.map((w) => ({ ...w, active: w.index === windowIndex })),
      )
      if (preferredPaneID !== null)
        setPanes((prev) =>
          prev.map((p) => ({ ...p, active: p.paneId === preferredPaneID })),
        )
      const gen = selectInFlightRef.current
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/select-window`,
        { method: 'POST', body: JSON.stringify({ index: windowIndex }) },
      )
        .then(() => {
          // Prompt refresh so the override is confirmed without waiting for
          // the next watchtower event.  selectInFlightRef stays > 0 until
          // refreshInspector sees the server has caught up.
          void refreshInspector(active, { background: true })
        })
        .catch((error) => {
          if (selectInFlightRef.current === gen) selectInFlightRef.current = 0
          pendingSelectMutationKindRef.current = null
          const msg =
            error instanceof Error ? error.message : 'failed to switch window'
          setInspectorError(msg)
          pushErrorToast('Switch Window', msg)
          setActiveWindowIndexOverride(null)
          setActivePaneIDOverride(null)
          void refreshInspector(active, { background: true })
        })
    },
    [api, panes, pushErrorToast, refreshInspector, tabsStateRef],
  )

  const selectPane = useCallback(
    (paneID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active || !paneID.trim()) return
      if (isPendingSplitPaneID(paneID)) return
      if (activePaneOverrideRef.current === paneID) return
      const paneInfo = panes.find((p) => p.paneId === paneID)
      setInspectorError('')
      inspectorGenerationRef.current += 1
      if (inspectorLoadingRef.current) {
        inspectorLoadingRef.current = false
        setInspectorLoading(false)
      }
      pendingSelectMutationKindRef.current = 'pane'
      selectInFlightRef.current += 1
      setActivePaneIDOverride(paneID)
      if (paneInfo) setActiveWindowIndexOverride(paneInfo.windowIndex)
      setPanes((prev) =>
        prev.map((p) => ({ ...p, active: p.paneId === paneID })),
      )
      if (paneInfo)
        setWindows((prev) =>
          prev.map((w) => ({ ...w, active: w.index === paneInfo.windowIndex })),
        )
      const gen = selectInFlightRef.current
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/select-pane`,
        { method: 'POST', body: JSON.stringify({ paneId: paneID }) },
      )
        .then(() => {
          void refreshInspector(active, { background: true })
        })
        .catch((error) => {
          if (selectInFlightRef.current === gen) selectInFlightRef.current = 0
          pendingSelectMutationKindRef.current = null
          const msg =
            error instanceof Error ? error.message : 'failed to switch pane'
          setInspectorError(msg)
          pushErrorToast('Switch Pane', msg)
          setActiveWindowIndexOverride(null)
          setActivePaneIDOverride(null)
          void refreshInspector(active, { background: true })
        })
    },
    [api, panes, pushErrorToast, refreshInspector, tabsStateRef],
  )

  const createWindow = useCallback(() => {
    const active = tabsStateRef.current.activeSession
    if (!active) return
    const changedAt = new Date().toISOString()
    const nextIdx = windows.reduce((h, w) => Math.max(h, w.index), -1) + 1
    const pendingPaneID = buildPendingSplitPaneID(active, nextIdx, 0)
    removePendingWindowClose(pendingCloseWindowsRef.current, active, nextIdx)
    addPendingWindowCreate(pendingCreateWindowsRef.current, active, nextIdx)
    setPendingWindowPaneFloor(
      pendingWindowPaneFloorsRef.current,
      active,
      nextIdx,
      1,
    )
    setInspectorError('')
    setSessions((prev) =>
      prev.map((item) =>
        item.name === active
          ? {
              ...item,
              windows: item.windows + 1,
              panes: item.panes + 1,
              activityAt: changedAt,
            }
          : item,
      ),
    )
    setWindows((prev) => [
      ...prev
        .filter((w) => w.index !== nextIdx)
        .map((w) => ({ ...w, active: false })),
      {
        session: active,
        index: nextIdx,
        name: 'new',
        displayName: 'new',
        tmuxWindowId: undefined,
        active: true,
        panes: 1,
      },
    ])
    setPanes((prev) => [
      ...prev
        .filter((paneInfo) => paneInfo.paneId !== pendingPaneID)
        .map((p) => ({ ...p, active: false })),
      {
        session: active,
        windowIndex: nextIdx,
        paneIndex: 0,
        paneId: pendingPaneID,
        title: 'new',
        active: true,
        tty: '',
        hasUnread: false,
      },
    ])
    setActiveWindowIndexOverride(nextIdx)
    setActivePaneIDOverride(pendingPaneID)
    void api<void>(
      `/api/tmux/sessions/${encodeURIComponent(active)}/new-window`,
      { method: 'POST', body: '{}' },
    )
      .then(() => {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          active,
          nextIdx,
        )
        clearPendingWindowPaneFloor(
          pendingWindowPaneFloorsRef.current,
          active,
          nextIdx,
        )
        void refreshInspector(active, { background: true })
        void refreshSessions()
      })
      .catch((error) => {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          active,
          nextIdx,
        )
        if (error instanceof GuardrailConfirmError) {
          const rules = error.decision.matchedRules
          requestGuardrailConfirm(
            rules[0]?.name ?? '',
            error.decision.message,
            () => {
              void api<void>(
                `/api/tmux/sessions/${encodeURIComponent(active)}/new-window`,
                {
                  method: 'POST',
                  body: '{}',
                  headers: { 'X-Sentinel-Guardrail-Confirm': 'true' },
                },
              )
                .then(() => {
                  removePendingWindowCreate(
                    pendingCreateWindowsRef.current,
                    active,
                    nextIdx,
                  )
                  clearPendingWindowPaneFloor(
                    pendingWindowPaneFloorsRef.current,
                    active,
                    nextIdx,
                  )
                  void refreshInspector(active, { background: true })
                  void refreshSessions()
                })
                .catch((retryError) => {
                  const retryMsg =
                    retryError instanceof Error
                      ? retryError.message
                      : 'failed to create window'
                  pushErrorToast('New Window', retryMsg)
                  void refreshInspector(active, { background: true })
                  void refreshSessions()
                })
            },
          )
          void refreshInspector(active, { background: true })
          void refreshSessions()
          return
        }
        const msg =
          error instanceof Error ? error.message : 'failed to create window'
        setInspectorError(msg)
        pushErrorToast('New Window', msg)
        void refreshInspector(active, { background: true })
        void refreshSessions()
      })
  }, [
    api,
    pushErrorToast,
    refreshInspector,
    refreshSessions,
    requestGuardrailConfirm,
    setSessions,
    tabsStateRef,
    windows,
  ])

  const closeWindow = useCallback(
    (windowIndex: number) => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      const removedPaneCount = panes.filter(
        (paneInfo) => paneInfo.windowIndex === windowIndex,
      ).length
      const changedAt = new Date().toISOString()
      removePendingWindowCreate(
        pendingCreateWindowsRef.current,
        active,
        windowIndex,
      )
      addPendingWindowClose(pendingCloseWindowsRef.current, active, windowIndex)
      clearPendingWindowPaneFloor(
        pendingWindowPaneFloorsRef.current,
        active,
        windowIndex,
      )
      const rem = windows.filter((w) => w.index !== windowIndex)
      const remP = panes.filter((p) => p.windowIndex !== windowIndex)
      const ord = [...rem].sort((a, b) => a.index - b.index)

      const currentActiveWindowIndex =
        activeWindowOverrideRef.current ??
        windows.find((w) => w.active)?.index ??
        null

      let nextWI: number | null = null
      if (
        currentActiveWindowIndex !== null &&
        ord.some((w) => w.index === currentActiveWindowIndex)
      )
        nextWI = currentActiveWindowIndex
      if (nextWI === null) {
        const h = ord.find((w) => w.index > windowIndex)
        nextWI = h ? h.index : (ord.at(-1)?.index ?? null)
      }
      let nextPI: string | null = null
      if (nextWI !== null) {
        const ap = remP.find((p) => p.windowIndex === nextWI && p.active)
        nextPI = ap
          ? ap.paneId
          : (remP.find((p) => p.windowIndex === nextWI)?.paneId ?? null)
      }
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                windows: Math.max(0, item.windows - 1),
                panes: Math.max(0, item.panes - Math.max(1, removedPaneCount)),
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows(rem.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-window`,
        { method: 'POST', body: JSON.stringify({ index: windowIndex }) },
      )
        .then(() => {
          removePendingWindowClose(
            pendingCloseWindowsRef.current,
            active,
            windowIndex,
          )
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
        .catch((error) => {
          removePendingWindowClose(
            pendingCloseWindowsRef.current,
            active,
            windowIndex,
          )
          if (error instanceof GuardrailConfirmError) {
            const rules = error.decision.matchedRules
            requestGuardrailConfirm(
              rules[0]?.name ?? '',
              error.decision.message,
              () => {
                void api<void>(
                  `/api/tmux/sessions/${encodeURIComponent(active)}/kill-window`,
                  {
                    method: 'POST',
                    body: JSON.stringify({ index: windowIndex }),
                    headers: { 'X-Sentinel-Guardrail-Confirm': 'true' },
                  },
                )
                  .then(() => {
                    removePendingWindowClose(
                      pendingCloseWindowsRef.current,
                      active,
                      windowIndex,
                    )
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
                  .catch((retryError) => {
                    const retryMsg =
                      retryError instanceof Error
                        ? retryError.message
                        : 'failed to close window'
                    pushErrorToast('Kill Window', retryMsg)
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
              },
            )
            void refreshInspector(active, { background: true })
            void refreshSessions()
            return
          }
          const msg =
            error instanceof Error ? error.message : 'failed to close window'
          setInspectorError(msg)
          pushErrorToast('Kill Window', msg)
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
    },
    [
      api,
      panes,
      pushErrorToast,
      refreshInspector,
      refreshSessions,
      requestGuardrailConfirm,
      setSessions,
      tabsStateRef,
      windows,
    ],
  )

  const closePane = useCallback(
    (paneID: string) => {
      const active = tabsStateRef.current.activeSession
      if (!active || !paneID.trim()) return
      if (isPendingSplitPaneID(paneID)) return
      const changedAt = new Date().toISOString()
      addPendingPaneClose(pendingClosePanesRef.current, active, paneID)
      const removed = panes.find((p) => p.paneId === paneID)
      const remP = panes.filter((p) => p.paneId !== paneID)
      const countByW = new Map<number, number>()
      for (const p of remP)
        countByW.set(p.windowIndex, (countByW.get(p.windowIndex) ?? 0) + 1)
      const remW = windows
        .filter((w) => countByW.has(w.index))
        .map((w) => ({ ...w, panes: countByW.get(w.index) ?? 0 }))
      const ord = [...remW].sort((a, b) => a.index - b.index)

      const currentActiveWindowIndex =
        activeWindowOverrideRef.current ??
        windows.find((w) => w.active)?.index ??
        null
      const currentActivePaneID =
        activePaneOverrideRef.current ??
        panes.find((p) => p.active)?.paneId ??
        null

      let nextWI: number | null = null
      if (
        currentActiveWindowIndex !== null &&
        ord.some((w) => w.index === currentActiveWindowIndex)
      )
        nextWI = currentActiveWindowIndex
      if (
        nextWI === null &&
        removed &&
        ord.some((w) => w.index === removed.windowIndex)
      )
        nextWI = removed.windowIndex
      if (nextWI === null) nextWI = ord.at(0)?.index ?? null
      let nextPI: string | null = null
      if (
        currentActivePaneID !== null &&
        remP.some((p) => p.paneId === currentActivePaneID)
      )
        nextPI = currentActivePaneID
      if (nextPI === null && nextWI !== null) {
        const ap = remP.find((p) => p.windowIndex === nextWI && p.active)
        nextPI = ap
          ? ap.paneId
          : (remP.find((p) => p.windowIndex === nextWI)?.paneId ?? null)
      }
      const removedWindow = removed
        ? !remW.some((w) => w.index === removed.windowIndex)
        : false
      if (removed && removedWindow) {
        removePendingWindowCreate(
          pendingCreateWindowsRef.current,
          active,
          removed.windowIndex,
        )
        addPendingWindowClose(
          pendingCloseWindowsRef.current,
          active,
          removed.windowIndex,
        )
      }
      if (removed) {
        clearPendingWindowPaneFloor(
          pendingWindowPaneFloorsRef.current,
          active,
          removed.windowIndex,
        )
      }
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                panes: Math.max(0, item.panes - 1),
                windows: removedWindow
                  ? Math.max(0, item.windows - 1)
                  : item.windows,
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows(remW.map((w) => ({ ...w, active: w.index === nextWI })))
      setPanes(remP.map((p) => ({ ...p, active: p.paneId === nextPI })))
      setActiveWindowIndexOverride(nextWI)
      setActivePaneIDOverride(nextPI)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/kill-pane`,
        { method: 'POST', body: JSON.stringify({ paneId: paneID }) },
      )
        .then(() => {
          removePendingPaneClose(pendingClosePanesRef.current, active, paneID)
          if (removedWindow && removed) {
            removePendingWindowClose(
              pendingCloseWindowsRef.current,
              active,
              removed.windowIndex,
            )
          }
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
        .catch((error) => {
          removePendingPaneClose(pendingClosePanesRef.current, active, paneID)
          if (removedWindow && removed) {
            removePendingWindowClose(
              pendingCloseWindowsRef.current,
              active,
              removed.windowIndex,
            )
          }
          if (error instanceof GuardrailConfirmError) {
            const rules = error.decision.matchedRules
            requestGuardrailConfirm(
              rules[0]?.name ?? '',
              error.decision.message,
              () => {
                void api<void>(
                  `/api/tmux/sessions/${encodeURIComponent(active)}/kill-pane`,
                  {
                    method: 'POST',
                    body: JSON.stringify({ paneId: paneID }),
                    headers: { 'X-Sentinel-Guardrail-Confirm': 'true' },
                  },
                )
                  .then(() => {
                    removePendingPaneClose(
                      pendingClosePanesRef.current,
                      active,
                      paneID,
                    )
                    if (removedWindow && removed) {
                      removePendingWindowClose(
                        pendingCloseWindowsRef.current,
                        active,
                        removed.windowIndex,
                      )
                    }
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
                  .catch((retryError) => {
                    const retryMsg =
                      retryError instanceof Error
                        ? retryError.message
                        : 'failed to close pane'
                    pushErrorToast('Kill Pane', retryMsg)
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
              },
            )
            void refreshInspector(active, { background: true })
            void refreshSessions()
            return
          }
          const msg =
            error instanceof Error ? error.message : 'failed to close pane'
          setInspectorError(msg)
          pushErrorToast('Kill Pane', msg)
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
    },
    [
      api,
      panes,
      pushErrorToast,
      refreshInspector,
      refreshSessions,
      requestGuardrailConfirm,
      setSessions,
      tabsStateRef,
      windows,
    ],
  )

  const splitPane = useCallback(
    (direction: 'vertical' | 'horizontal') => {
      const active = tabsStateRef.current.activeSession
      if (!active) return
      const changedAt = new Date().toISOString()

      const currentActiveWindowIndex =
        activeWindowOverrideRef.current ??
        windows.find((w) => w.active)?.index ??
        null
      const currentActivePaneID =
        activePaneOverrideRef.current ??
        panes.find((p) => p.active)?.paneId ??
        null

      const inWin =
        currentActiveWindowIndex === null
          ? []
          : panes.filter((p) => p.windowIndex === currentActiveWindowIndex)
      const targetID =
        inWin.find((p) => p.paneId === currentActivePaneID)?.paneId ??
        inWin.find((p) => p.active)?.paneId ??
        inWin[0]?.paneId
      if (!targetID) {
        pushErrorToast('Split Pane', 'no pane available to split')
        return
      }
      const target = panes.find((p) => p.paneId === targetID)
      if (!target) {
        pushErrorToast('Split Pane', 'target pane is not available')
        return
      }
      const expectedPaneFloor =
        (windows.find((windowInfo) => windowInfo.index === target.windowIndex)
          ?.panes ?? inWin.length) + 1
      const pendingPaneID = buildPendingSplitPaneID(
        active,
        target.windowIndex,
        inWin.length,
      )
      setPendingWindowPaneFloor(
        pendingWindowPaneFloorsRef.current,
        active,
        target.windowIndex,
        expectedPaneFloor,
      )
      setInspectorError('')
      setSessions((prev) =>
        prev.map((item) =>
          item.name === active
            ? {
                ...item,
                panes: item.panes + 1,
                activityAt: changedAt,
              }
            : item,
        ),
      )
      setWindows((prev) =>
        prev.map((w) => ({
          ...w,
          active: w.index === target.windowIndex,
          panes: w.index === target.windowIndex ? w.panes + 1 : w.panes,
        })),
      )
      setPanes((prev) => {
        const inWindow = prev.filter(
          (paneInfo) => paneInfo.windowIndex === target.windowIndex,
        )
        const nextPaneIndex =
          inWindow.reduce(
            (highest, paneInfo) => Math.max(highest, paneInfo.paneIndex),
            -1,
          ) + 1
        const withoutPending = prev.filter(
          (paneInfo) => paneInfo.paneId !== pendingPaneID,
        )
        return [
          ...withoutPending.map((p) => ({ ...p, active: false })),
          {
            session: active,
            windowIndex: target.windowIndex,
            paneIndex: nextPaneIndex,
            paneId: pendingPaneID,
            title: 'new',
            active: true,
            tty: '',
            hasUnread: false,
          },
        ]
      })
      setActiveWindowIndexOverride(target.windowIndex)
      setActivePaneIDOverride(pendingPaneID)
      void api<void>(
        `/api/tmux/sessions/${encodeURIComponent(active)}/split-pane`,
        {
          method: 'POST',
          body: JSON.stringify({ paneId: targetID, direction }),
        },
      )
        .then(() => {
          clearPendingWindowPaneFloor(
            pendingWindowPaneFloorsRef.current,
            active,
            target.windowIndex,
          )
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
        .catch((error) => {
          clearPendingWindowPaneFloor(
            pendingWindowPaneFloorsRef.current,
            active,
            target.windowIndex,
          )
          if (error instanceof GuardrailConfirmError) {
            const rules = error.decision.matchedRules
            requestGuardrailConfirm(
              rules[0]?.name ?? '',
              error.decision.message,
              () => {
                void api<void>(
                  `/api/tmux/sessions/${encodeURIComponent(active)}/split-pane`,
                  {
                    method: 'POST',
                    body: JSON.stringify({
                      paneId: targetID,
                      direction,
                    }),
                    headers: { 'X-Sentinel-Guardrail-Confirm': 'true' },
                  },
                )
                  .then(() => {
                    clearPendingWindowPaneFloor(
                      pendingWindowPaneFloorsRef.current,
                      active,
                      target.windowIndex,
                    )
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
                  .catch((retryError) => {
                    const retryMsg =
                      retryError instanceof Error
                        ? retryError.message
                        : 'failed to split pane'
                    pushErrorToast('Split Pane', retryMsg)
                    void refreshInspector(active, { background: true })
                    void refreshSessions()
                  })
              },
            )
            void refreshInspector(active, { background: true })
            void refreshSessions()
            return
          }
          const msg =
            error instanceof Error ? error.message : 'failed to split pane'
          setInspectorError(msg)
          pushErrorToast('Split Pane', msg)
          void refreshInspector(active, { background: true })
          void refreshSessions()
        })
    },
    [
      api,
      panes,
      pushErrorToast,
      refreshInspector,
      refreshSessions,
      requestGuardrailConfirm,
      setSessions,
      tabsStateRef,
      windows,
    ],
  )

  const renameWindow = useCallback(
    async (session: string, index: number, newName: string) => {
      const active = session.trim()
      if (!active) {
        setConnection('error', 'no active session')
        pushErrorToast('Rename Window', 'no active session')
        return
      }
      const sanitized = sanitizeTmuxWindowName(newName).trim()
      if (!sanitized) {
        pushErrorToast('Rename Window', 'window name required')
        return
      }
      setWindows((prev) =>
        prev.map((w) =>
          w.index === index
            ? { ...w, name: sanitized, displayName: sanitized }
            : w,
        ),
      )
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-window`,
          {
            method: 'POST',
            body: JSON.stringify({ index, name: sanitized }),
          },
        )
        pushSuccessToast('Rename Window', `window #${index} -> "${sanitized}"`)
        void refreshInspector(active, { background: true })
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename window'
        setInspectorError(msg)
        pushErrorToast('Rename Window', msg)
        void refreshInspector(active, { background: true })
      }
    },
    [
      api,
      pushErrorToast,
      pushSuccessToast,
      refreshInspector,
      setConnection,
      tabsStateRef,
    ],
  )

  const handleOpenRenameWindow = useCallback((windowInfo: WindowInfo) => {
    setRenameWindowTarget({
      session: windowInfo.session,
      index: windowInfo.index,
    })
    setRenameWindowValue(sanitizeTmuxWindowName(windowInfo.displayName))
    setRenameWindowDialogOpen(true)
  }, [])

  const handleSubmitRenameWindow = useCallback(() => {
    const target = renameWindowTarget
    if (target === null) return
    setRenameWindowDialogOpen(false)
    setRenameWindowTarget(null)
    void renameWindow(target.session, target.index, renameWindowValue)
  }, [renameWindow, renameWindowTarget, renameWindowValue])

  const renamePane = useCallback(
    async (session: string, paneID: string, title: string) => {
      const active = session.trim()
      if (!active) {
        setConnection('error', 'no active session')
        pushErrorToast('Rename Pane', 'no active session')
        return
      }
      if (isPendingSplitPaneID(paneID)) {
        pushErrorToast('Rename Pane', 'wait for pane creation to finish')
        return
      }
      const sanitized = sanitizeTmuxPaneTitle(title).trim()
      if (!sanitized) {
        pushErrorToast('Rename Pane', 'pane title required')
        return
      }
      setPanes((prev) =>
        prev.map((p) => (p.paneId === paneID ? { ...p, title: sanitized } : p)),
      )
      try {
        await api<void>(
          `/api/tmux/sessions/${encodeURIComponent(active)}/rename-pane`,
          {
            method: 'POST',
            body: JSON.stringify({ paneId: paneID, title: sanitized }),
          },
        )
        pushSuccessToast('Rename Pane', `pane ${paneID} renamed`)
        void refreshInspector(active, { background: true })
      } catch (error) {
        const msg =
          error instanceof Error ? error.message : 'failed to rename pane'
        setInspectorError(msg)
        pushErrorToast('Rename Pane', msg)
        void refreshInspector(active, { background: true })
      }
    },
    [
      api,
      pushErrorToast,
      pushSuccessToast,
      refreshInspector,
      setConnection,
      tabsStateRef,
    ],
  )

  const handleOpenRenamePane = useCallback((paneInfo: PaneInfo) => {
    const initialTitle =
      paneInfo.title.trim() !== '' ? paneInfo.title : paneInfo.paneId
    setRenamePaneTarget({
      session: paneInfo.session,
      paneID: paneInfo.paneId,
    })
    setRenamePaneValue(sanitizeTmuxPaneTitle(initialTitle))
    setRenamePaneDialogOpen(true)
  }, [])

  const handleSubmitRenamePane = useCallback(() => {
    const target = renamePaneTarget
    if (target === null) return
    setRenamePaneDialogOpen(false)
    setRenamePaneTarget(null)
    void renamePane(target.session, target.paneID, renamePaneValue)
  }, [renamePane, renamePaneTarget, renamePaneValue])

  return {
    // State
    windows,
    panes,
    activeWindowIndexOverride,
    activePaneIDOverride,
    inspectorLoading,
    inspectorError,
    renameWindowDialogOpen,
    renameWindowValue,
    renamePaneDialogOpen,
    renamePaneValue,
    // Refs (needed by other hooks)
    pendingCreateSessionsRef,
    // Actions
    refreshInspector,
    reorderWindows,
    selectWindow,
    selectPane,
    createWindow,
    closeWindow,
    splitPane,
    closePane,
    handleOpenRenameWindow,
    handleSubmitRenameWindow,
    handleOpenRenamePane,
    handleSubmitRenamePane,
    setRenameWindowDialogOpen,
    setRenameWindowTarget,
    setRenameWindowValue,
    setRenamePaneDialogOpen,
    setRenamePaneTarget,
    setRenamePaneValue,
    setWindows,
    setPanes,
    setActiveWindowIndexOverride,
    setActivePaneIDOverride,
    setInspectorError,
    setInspectorLoading,
    applySessionActivityPatches,
    applyInspectorProjectionPatches,
    mergeInspectorSnapshotWithPending,
    clearPendingInspectorSessionState,
  }
}
