import type { MutableRefObject } from 'react'
import type {
  PaneInfo,
  RecoveryJob,
  RecoverySession,
  RecoverySnapshotView,
  Session,
  TimelineEvent,
  WindowInfo,
} from '@/types'
import type { MergePendingInspectorResult } from '@/lib/tmuxInspectorOptimistic'
import type {
  SessionActivityPatch,
  SessionPatchApplyResult,
} from '@/lib/tmuxSessionEvents'
import type { TabsAction, TabsState } from '@/tabsReducer'

// ---------------------------------------------------------------------------
// Shared ref types created in TmuxPage and passed to hooks
// ---------------------------------------------------------------------------

export type PresenceSocketRef = MutableRefObject<WebSocket | null>
export type TabsStateRef = MutableRefObject<TabsState>
export type SessionsRef = MutableRefObject<Array<Session>>

// ---------------------------------------------------------------------------
// Patch / delta types (previously inline in tmux.tsx)
// ---------------------------------------------------------------------------

export type SeenAckMessage = {
  eventId?: number
  type?: string
  requestId?: string
  globalRev?: number
  sessionPatches?: Array<SessionActivityPatch>
  inspectorPatches?: Array<InspectorSessionPatch>
}

export type SeenCommandPayload = {
  session: string
  scope: 'pane' | 'window' | 'session'
  paneId?: string
  windowIndex?: number
}

export type InspectorWindowPatch = {
  session?: string
  index?: number
  name?: string
  active?: boolean
  panes?: number
  unreadPanes?: number
  hasUnread?: boolean
  rev?: number
  activityAt?: string
}

export type InspectorPanePatch = {
  session?: string
  windowIndex?: number
  paneIndex?: number
  paneId?: string
  title?: string
  active?: boolean
  tty?: string
  currentPath?: string
  startCommand?: string
  currentCommand?: string
  tailPreview?: string
  revision?: number
  seenRevision?: number
  hasUnread?: boolean
  changedAt?: string
}

export type InspectorSessionPatch = {
  session?: string
  windows?: Array<InspectorWindowPatch>
  panes?: Array<InspectorPanePatch>
}

export type ActivityDeltaChange = {
  id?: number
  globalRev?: number
  entityType?: string
  session?: string
  windowIndex?: number
  paneId?: string
  changeKind?: string
  changedAt?: string
}

export type ActivityDeltaResponse = {
  since?: number
  limit?: number
  globalRev?: number
  overflow?: boolean
  changes?: Array<ActivityDeltaChange>
  sessionPatches?: Array<SessionActivityPatch>
  inspectorPatches?: Array<InspectorSessionPatch>
}

export type RecoveryOverviewCache = {
  sessions: Array<RecoverySession>
  jobs: Array<RecoveryJob>
}

export type TmuxTimelineCache = {
  events: Array<TimelineEvent>
  hasMore: boolean
}

// ---------------------------------------------------------------------------
// Runtime metrics
// ---------------------------------------------------------------------------

export type RuntimeMetrics = {
  wsMessages: number
  wsReconnects: number
  wsOpenCount: number
  wsCloseCount: number
  sessionsRefreshCount: number
  inspectorRefreshCount: number
  recoveryRefreshCount: number
  fallbackRefreshCount: number
  deltaSyncCount: number
  deltaSyncErrors: number
  deltaOverflowCount: number
}

// ---------------------------------------------------------------------------
// Common callback types used across hooks
// ---------------------------------------------------------------------------

export type DispatchTabs = (action: TabsAction) => void

export type PushToastFn = (entry: {
  level: 'error' | 'success' | 'info'
  title: string
  message: string
}) => void

export type ApiFunction = <T>(path: string, init?: RequestInit) => Promise<T>

// ---------------------------------------------------------------------------
// Shared hook return types
// ---------------------------------------------------------------------------

export type RecoveryState = {
  recoverySessions: Array<RecoverySession>
  recoveryJobs: Array<RecoveryJob>
  recoveryDialogOpen: boolean
  recoverySnapshots: Array<{
    id: number
    capturedAt: string
    windows: number
    panes: number
  }>
  selectedRecoverySession: string | null
  selectedSnapshotID: number | null
  selectedSnapshot: RecoverySnapshotView | null
  recoveryLoading: boolean
  recoveryBusy: boolean
  recoveryError: string
  restoreMode: 'safe' | 'confirm' | 'full'
  restoreConflictPolicy: 'rename' | 'replace' | 'skip'
  restoreTargetSession: string
}

export type RecoveryActions = {
  setRecoveryDialogOpen: (open: boolean) => void
  setSelectedRecoverySession: (session: string | null) => void
  setRestoreTargetSession: (session: string) => void
  setRestoreMode: (mode: 'safe' | 'confirm' | 'full') => void
  setRestoreConflictPolicy: (policy: 'rename' | 'replace' | 'skip') => void
  refreshRecovery: (options?: { quiet?: boolean }) => Promise<void>
  loadRecoverySnapshot: (snapshotID: number) => Promise<void>
  loadRecoverySnapshots: (sessionName: string) => Promise<void>
  restoreSelectedSnapshot: () => Promise<void>
  archiveRecoverySession: (sessionName: string) => Promise<void>
  pollRecoveryJob: (jobID: string) => void
}

export type TimelineState = {
  timelineOpen: boolean
  timelineEvents: Array<TimelineEvent>
  timelineHasMore: boolean
  timelineLoading: boolean
  timelineError: string
  timelineQuery: string
  timelineSeverity: string
  timelineEventType: string
  timelineSessionFilter: string
}

export type TimelineActions = {
  setTimelineOpen: (open: boolean) => void
  setTimelineQuery: (query: string) => void
  setTimelineSeverity: (severity: string) => void
  setTimelineEventType: (eventType: string) => void
  setTimelineSessionFilter: (filter: string) => void
  loadTimeline: (options?: { quiet?: boolean }) => Promise<void>
}

export type InspectorState = {
  windows: Array<WindowInfo>
  panes: Array<PaneInfo>
  activeWindowIndexOverride: number | null
  activePaneIDOverride: string | null
  inspectorLoading: boolean
  inspectorError: string
}

export type InspectorActions = {
  refreshInspector: (
    target: string,
    options?: { background?: boolean },
  ) => Promise<void>
  selectWindow: (windowIndex: number) => void
  selectPane: (paneID: string) => void
  createWindow: () => void
  closeWindow: (windowIndex: number) => void
  splitPane: (direction: 'vertical' | 'horizontal') => void
  closePane: (paneID: string) => void
  handleOpenRenameWindow: (windowInfo: WindowInfo) => void
  handleSubmitRenameWindow: () => void
  handleOpenRenamePane: (paneInfo: PaneInfo) => void
  handleSubmitRenamePane: () => void
  renameWindowDialogOpen: boolean
  renameWindowValue: string
  setRenameWindowValue: (value: string) => void
  setRenameWindowDialogOpen: (open: boolean) => void
  setRenameWindowIndex: (index: number | null) => void
  renamePaneDialogOpen: boolean
  renamePaneValue: string
  setRenamePaneValue: (value: string) => void
  setRenamePaneDialogOpen: (open: boolean) => void
  setRenamePaneID: (id: string | null) => void
  applySessionActivityPatches: (
    rawPatches: Array<SessionActivityPatch> | undefined,
  ) => SessionPatchApplyResult
  applyInspectorProjectionPatches: (
    rawPatches: Array<InspectorSessionPatch> | undefined,
  ) => boolean
  setWindows: React.Dispatch<React.SetStateAction<Array<WindowInfo>>>
  setPanes: React.Dispatch<React.SetStateAction<Array<PaneInfo>>>
  setActiveWindowIndexOverride: (index: number | null) => void
  setActivePaneIDOverride: (id: string | null) => void
  setInspectorError: (error: string) => void
  setInspectorLoading: (loading: boolean) => void
  mergeInspectorSnapshotWithPending: (
    session: string,
    sourceWindows: Array<WindowInfo>,
    sourcePanes: Array<PaneInfo>,
  ) => MergePendingInspectorResult
  clearPendingInspectorSessionState: (session: string) => void
}

// ---------------------------------------------------------------------------
// Helpers hoisted to module level
// ---------------------------------------------------------------------------

export function asNonNegativeInt(
  value: number | undefined,
  fallback: number,
): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Math.trunc(value)
    : fallback
}

export function asNonNegativeInt64(
  value: number | undefined,
  fallback: number,
): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Math.trunc(value)
    : fallback
}

export function isTmuxBinaryMissingMessage(message: string): boolean {
  const normalized = message.trim().toLowerCase()
  return normalized.includes('tmux binary not found')
}

export function sameWindowProjection(
  left: Array<WindowInfo>,
  right: Array<WindowInfo>,
): boolean {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i]
    const b = right[i]
    if (
      a.session !== b.session ||
      a.index !== b.index ||
      a.name !== b.name ||
      a.active !== b.active ||
      a.panes !== b.panes ||
      (a.unreadPanes ?? 0) !== (b.unreadPanes ?? 0) ||
      (a.hasUnread ?? false) !== (b.hasUnread ?? false) ||
      (a.rev ?? 0) !== (b.rev ?? 0) ||
      (a.activityAt ?? '') !== (b.activityAt ?? '')
    ) {
      return false
    }
  }
  return true
}

export function samePaneProjection(
  left: Array<PaneInfo>,
  right: Array<PaneInfo>,
): boolean {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i += 1) {
    const a = left[i]
    const b = right[i]
    if (
      a.session !== b.session ||
      a.windowIndex !== b.windowIndex ||
      a.paneIndex !== b.paneIndex ||
      a.paneId !== b.paneId ||
      a.title !== b.title ||
      a.active !== b.active ||
      a.tty !== b.tty ||
      (a.tailPreview ?? '') !== (b.tailPreview ?? '') ||
      (a.revision ?? 0) !== (b.revision ?? 0) ||
      (a.seenRevision ?? 0) !== (b.seenRevision ?? 0) ||
      (a.hasUnread ?? false) !== (b.hasUnread ?? false) ||
      (a.changedAt ?? '') !== (b.changedAt ?? '')
    ) {
      return false
    }
  }
  return true
}

export function resolvePresenceTerminalID(generateId: () => string): string {
  const key = 'sentinel.tmux.presence.terminalId'
  const fromStorage = window.sessionStorage.getItem(key)
  if (fromStorage && fromStorage.trim() !== '') {
    return fromStorage
  }

  const generated = generateId()
  window.sessionStorage.setItem(key, generated)
  return generated
}
