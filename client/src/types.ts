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
  scope: 'action'
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

export type GuardrailAudit = {
  id: number
  ruleId: string
  decision: string
  action: string
  command: string
  sessionName: string
  windowIndex: number
  paneId: string
  override: boolean
  reason: string
  metadata: string
  createdAt: string
}

export type GuardrailAuditResponse = {
  audit: Array<GuardrailAudit>
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

export type OpsServiceAction =
  | 'start'
  | 'stop'
  | 'restart'
  | 'enable'
  | 'disable'

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

export type OpsActivityEvent = {
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

export type OpsActivityResponse = {
  events: Array<OpsActivityEvent>
  hasMore: boolean
}

export type OpsRunbookStepType = 'run' | 'script' | 'approval'

export type OpsRunbookStep = {
  type: OpsRunbookStepType
  title: string
  command?: string
  script?: string
  description?: string
  continueOnError?: boolean
  timeout?: number
  retries?: number
  retryDelay?: number
}

export type RunbookParameterType = 'string' | 'number' | 'boolean' | 'select'

export type RunbookParameter = {
  name: string
  label: string
  type: RunbookParameterType
  default: string
  required: boolean
  options?: Array<string>
}

export type OpsRunbook = {
  id: string
  name: string
  description: string
  enabled: boolean
  webhookURL?: string
  parameters?: Array<RunbookParameter>
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
  parametersUsed?: Record<string, string>
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

export type SuggestedRunbooksResponse = {
  runbooks: Array<OpsRunbook>
}

export type OpsRunbookRunResponse = {
  job: OpsRunbookRun
  timelineEvent?: OpsActivityEvent
  globalRev?: number
}

export type OpsServiceActionResponse = {
  service: OpsServiceStatus
  services: Array<OpsServiceStatus>
  overview: OpsOverview
  timelineEvent?: OpsActivityEvent
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
  unitType: string
  description: string
  activeState: string
  enabledState: string
  manager: string
  scope: string
}

export type OpsDiscoverServicesResponse = {
  services: Array<OpsAvailableService>
}

export type OpsBrowsedService = {
  unit: string
  unitType: string
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
  timelineEvent?: OpsActivityEvent
  alerts?: Array<OpsAlert>
  globalRev: number
}

export type OpsUnitLogsResponse = {
  unit: string
  lines: number
  output: string
}

export type WebhookSettings = {
  url: string
  events: Array<string>
}

export type WebhookTestResponse = {
  success: boolean
  message: string
}

export type OpsWsMessage =
  | { type: 'ops.overview.updated'; payload: { overview: OpsOverview } }
  | {
      type: 'ops.services.updated'
      payload: { services: Array<OpsServiceStatus> }
    }
  | { type: 'ops.alerts.updated'; payload: { alerts: Array<OpsAlert> } }
  | {
      type: 'ops.activity.updated'
      payload: {
        event?: OpsActivityEvent
        events?: Array<OpsActivityEvent>
      }
    }
  | { type: 'ops.metrics.updated'; payload: { metrics: OpsHostMetrics } }
  | { type: 'ops.job.updated'; payload: { job: OpsRunbookRun } }
  | { type: 'ops.schedule.updated'; payload: Record<string, unknown> }
