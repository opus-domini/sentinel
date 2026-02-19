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
  scope: 'action' | 'command'
  pattern: string
  mode: 'allow' | 'warn' | 'confirm' | 'block'
  severity: 'info' | 'warn' | 'error'
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

export type OpsServiceAction = 'start' | 'stop' | 'restart'

export type OpsServiceStatus = {
  name: string
  displayName: string
  manager: string
  scope: string
  unit: string
  exists: boolean
  enabledState: string
  activeState: string
  lastRunState?: string
  updatedAt: string
}

export type OpsServiceInspect = {
  service: OpsServiceStatus
  summary: string
  properties?: Record<string, string>
  output?: string
  checkedAt: string
}

export type OpsAlertStatus = 'open' | 'acked' | 'resolved'

export type OpsAlert = {
  id: number
  dedupeKey: string
  source: string
  resource: string
  title: string
  message: string
  severity: string
  status: OpsAlertStatus
  occurrences: number
  metadata: string
  firstSeenAt: string
  lastSeenAt: string
  ackedAt?: string
  resolvedAt?: string
}

export type OpsTimelineEvent = {
  id: number
  source: string
  eventType: string
  severity: string
  resource: string
  message: string
  details: string
  metadata: string
  createdAt: string
}

export type OpsOverview = {
  host: {
    hostname: string
    os: string
    arch: string
    cpus: number
    goVersion: string
  }
  sentinel: {
    pid: number
    uptimeSec: number
  }
  services: {
    total: number
    active: number
    failed: number
  }
  updatedAt: string
}

export type OpsOverviewResponse = {
  overview: OpsOverview
}

export type OpsServicesResponse = {
  services: Array<OpsServiceStatus>
}

export type OpsAlertsResponse = {
  alerts: Array<OpsAlert>
}

export type OpsTimelineResponse = {
  events: Array<OpsTimelineEvent>
  hasMore: boolean
}

export type OpsRunbookStep = {
  type: string
  title: string
  command?: string
  check?: string
  description?: string
}

export type OpsRunbook = {
  id: string
  name: string
  description: string
  enabled: boolean
  steps: Array<OpsRunbookStep>
  createdAt: string
  updatedAt: string
}

export type OpsRunbookStepResult = {
  stepIndex: number
  title: string
  type: string
  output: string
  error: string
  durationMs: number
}

export type OpsRunbookRun = {
  id: string
  runbookId: string
  runbookName: string
  status: string
  totalSteps: number
  completedSteps: number
  currentStep: string
  error: string
  stepResults: Array<OpsRunbookStepResult>
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export type OpsSchedule = {
  id: string
  runbookId: string
  name: string
  scheduleType: string
  cronExpr: string
  timezone: string
  runAt: string
  enabled: boolean
  lastRunAt: string
  lastRunStatus: string
  nextRunAt: string
  createdAt: string
  updatedAt: string
}

export type OpsRunbooksResponse = {
  runbooks: Array<OpsRunbook>
  jobs: Array<OpsRunbookRun>
  schedules: Array<OpsSchedule>
}

export type OpsRunbookRunResponse = {
  job: OpsRunbookRun
  timelineEvent?: OpsTimelineEvent
  globalRev?: number
}

export type OpsServiceActionResponse = {
  service: OpsServiceStatus
  services: Array<OpsServiceStatus>
  overview: OpsOverview
  timelineEvent?: OpsTimelineEvent
  alerts?: Array<OpsAlert>
  globalRev: number
}

export type OpsServiceStatusResponse = {
  status: OpsServiceInspect
}

export type OpsServiceLogsResponse = {
  service: string
  lines: number
  output: string
}

export type OpsHostMetrics = {
  cpuPercent: number
  memUsedBytes: number
  memTotalBytes: number
  memPercent: number
  diskUsedBytes: number
  diskTotalBytes: number
  diskPercent: number
  loadAvg1: number
  loadAvg5: number
  loadAvg15: number
  numGoroutines: number
  goMemAllocMB: number
  collectedAt: string
}

export type OpsMetricsResponse = {
  metrics: OpsHostMetrics
}

export type OpsCustomServiceWrite = {
  name: string
  displayName: string
  manager: string
  unit: string
  scope: string
}

export type OpsAvailableService = {
  unit: string
  description: string
  activeState: string
  manager: string
  scope: string
}

export type OpsDiscoverServicesResponse = {
  services: Array<OpsAvailableService>
}

export type OpsBrowsedService = {
  unit: string
  description: string
  activeState: string
  enabledState: string
  manager: string
  scope: string
  tracked: boolean
  trackedName?: string
}

export type OpsBrowseServicesResponse = {
  services: Array<OpsBrowsedService>
}

export type OpsUnitActionResponse = {
  overview: OpsOverview
  timelineEvent?: OpsTimelineEvent
  alerts?: Array<OpsAlert>
  globalRev: number
}

export type OpsUnitLogsResponse = {
  unit: string
  lines: number
  output: string
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

export type OpsWsMessage =
  | { type: 'ops.overview.updated'; payload: { overview: OpsOverview } }
  | {
      type: 'ops.services.updated'
      payload: { services: Array<OpsServiceStatus> }
    }
  | { type: 'ops.alerts.updated'; payload: { alerts: Array<OpsAlert> } }
  | {
      type: 'ops.timeline.updated'
      payload: {
        event?: OpsTimelineEvent
        events?: Array<OpsTimelineEvent>
      }
    }
  | { type: 'ops.metrics.updated'; payload: { metrics: OpsHostMetrics } }
  | { type: 'ops.job.updated'; payload: { job: OpsRunbookRun } }
  | { type: 'ops.schedule.updated'; payload: Record<string, unknown> }
