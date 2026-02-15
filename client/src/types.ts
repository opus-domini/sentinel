export type Session = {
  name: string
  windows: number
  panes: number
  attached: number
  createdAt: string
  activityAt: string
  command: string
  hash: string
  lastContent: string
  icon: string
  unreadWindows?: number
  unreadPanes?: number
  rev?: number
}

export type ConnectionState =
  | 'connected'
  | 'connecting'
  | 'disconnected'
  | 'error'

export type SessionsResponse = {
  sessions: Array<Session>
}

export type WindowInfo = {
  session: string
  index: number
  name: string
  active: boolean
  panes: number
  unreadPanes?: number
  hasUnread?: boolean
  rev?: number
  activityAt?: string
}

export type PaneInfo = {
  session: string
  windowIndex: number
  paneIndex: number
  paneId: string
  title: string
  active: boolean
  tty: string
  tailPreview?: string
  revision?: number
  seenRevision?: number
  hasUnread?: boolean
  changedAt?: string
}

export type WindowsResponse = {
  windows: Array<WindowInfo>
}

export type PanesResponse = {
  panes: Array<PaneInfo>
}

export type TimelineSeverity = 'info' | 'warn' | 'error' | ''

export type TimelineEvent = {
  id: number
  session: string
  windowIndex: number
  paneId: string
  eventType: string
  severity: TimelineSeverity
  command: string
  cwd: string
  durationMs: number
  summary: string
  details: string
  marker: string
  metadata: Record<string, unknown> | null
  createdAt: string
}

export type TimelineResponse = {
  events: Array<TimelineEvent>
  hasMore: boolean
}

export type GuardrailRule = {
  id: string
  name: string
  scope: 'action' | 'command' | string
  pattern: string
  mode: 'allow' | 'warn' | 'confirm' | 'block' | string
  severity: 'info' | 'warn' | 'error' | string
  message: string
  enabled: boolean
  priority: number
  createdAt: string
  updatedAt: string
}

export type GuardrailRulesResponse = {
  rules: Array<GuardrailRule>
}

export type StorageResourceStat = {
  resource: string
  label: string
  rows: number
  approxBytes: number
}

export type StorageStatsResponse = {
  databaseBytes: number
  walBytes: number
  shmBytes: number
  totalBytes: number
  resources: Array<StorageResourceStat>
  collectedAt: string
}

export type StorageFlushResult = {
  resource: string
  removedRows: number
}

export type StorageFlushResponse = {
  results: Array<StorageFlushResult>
  flushedAt: string
}

export type TerminalConnection = {
  id: string
  tty: string
  user: string
  processCount: number
  leaderPid: number
  command: string
  args: string
}

export type TerminalsResponse = {
  terminals: Array<TerminalConnection>
}

export type TerminalProcess = {
  pid: number
  ppid: number
  user: string
  command: string
  args: string
  cpu: number
  mem: number
}

export type SystemTerminalDetailResponse = {
  tty: string
  processes: Array<TerminalProcess>
}

export type RecoverySessionState =
  | 'running'
  | 'killed'
  | 'restoring'
  | 'restored'
  | 'archived'

export type RecoverySession = {
  name: string
  state: RecoverySessionState
  latestSnapshotId: number
  snapshotHash: string
  snapshotAt: string
  lastBootId: string
  lastSeenAt: string
  killedAt?: string
  restoredAt?: string
  archivedAt?: string
  restoreError: string
  windows: number
  panes: number
}

export type RecoveryJobStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'partial'

export type RecoveryJob = {
  id: string
  sessionName: string
  targetSession: string
  snapshotId: number
  mode: 'safe' | 'confirm' | 'full'
  conflictPolicy: 'rename' | 'replace' | 'skip'
  status: RecoveryJobStatus
  totalSteps: number
  completedSteps: number
  currentStep: string
  error: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export type RecoveryOverview = {
  bootId: string
  lastBootId: string
  lastCollectAt: string
  lastBootChange: string
  killedSessions: Array<RecoverySession>
  runningJobs: Array<RecoveryJob>
}

export type RecoveryOverviewResponse = {
  overview: RecoveryOverview
}

export type RecoverySessionsResponse = {
  sessions: Array<RecoverySession>
}

export type RecoverySnapshotMeta = {
  id: number
  sessionName: string
  bootId: string
  stateHash: string
  capturedAt: string
  activeWindow: number
  activePaneId: string
  windows: number
  panes: number
  payloadJson: string
}

export type RecoverySnapshotsResponse = {
  snapshots: Array<RecoverySnapshotMeta>
}

export type RecoveryWindowSnapshot = {
  index: number
  name: string
  active: boolean
  panes: number
  layout: string
}

export type RecoveryPaneSnapshot = {
  windowIndex: number
  paneIndex: number
  title: string
  active: boolean
  currentPath: string
  startCommand: string
  currentCommand: string
  lastContent: string
}

export type RecoverySnapshotPayload = {
  sessionName: string
  capturedAt: string
  bootId: string
  attached: number
  activeWindow: number
  activePaneId: string
  windows: Array<RecoveryWindowSnapshot>
  panes: Array<RecoveryPaneSnapshot>
}

export type RecoverySnapshotView = {
  meta: RecoverySnapshotMeta
  payload: RecoverySnapshotPayload
}

export type RecoverySnapshotResponse = {
  snapshot: RecoverySnapshotView
}

export type RecoveryJobResponse = {
  job: RecoveryJob
}
