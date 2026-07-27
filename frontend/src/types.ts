export type Session = {
  name: string
  sortOrder?: number
  windows: number
  panes: number
  attached: number
  createdAt: string
  activityAt: string
  command: string
  hash: string
  lastContent: string
  icon: string
  user?: string
  unreadWindows?: number
  unreadPanes?: number
  rev?: number
}

export type SessionPreset = {
  name: string
  cwd: string
  icon: string
  user?: string
  sortOrder?: number
  createdAt: string
  updatedAt: string
  lastLaunchedAt: string
  launchCount: number
}

export type SessionPresetsResponse = {
  presets: Array<SessionPreset>
}

export type LaunchSessionPresetResponse = {
  name: string
  created: boolean
}

export type SessionLauncher = {
  id: string
  name: string
  cwd: string
  icon: string
  user?: string
  sortOrder?: number
  createdAt: string
  updatedAt: string
  lastUsedAt: string
  useCount: number
}

export type SessionLaunchersResponse = {
  launchers: Array<SessionLauncher>
}

export type LaunchSessionLauncherResponse = {
  launcherId: string
  name: string
  created: boolean
}

export type LauncherCwdMode = 'session' | 'active-pane' | 'fixed'

export type LauncherUserMode = 'session' | 'fixed'

export type TmuxLauncher = {
  id: string
  name: string
  icon: string
  command: string
  cwdMode: LauncherCwdMode
  cwdValue: string
  windowName: string
  userMode?: string
  userValue?: string
  sortOrder?: number
  createdAt: string
  updatedAt: string
  lastUsedAt: string
}

export type TmuxLaunchersResponse = {
  launchers: Array<TmuxLauncher>
}

export type LaunchTmuxLauncherResponse = {
  launcherId: string
  windowIndex: number
  paneId: string
  windowName: string
  managedWindowId: string
}

export type ConnectionState = 'connected' | 'connecting' | 'disconnected' | 'error'

export type SessionsResponse = {
  sessions: Array<Session>
}

export type WindowInfo = {
  session: string
  index: number
  name: string
  displayName: string
  displayIcon?: string
  tmuxWindowId?: string
  managed?: boolean
  managedWindowId?: string
  launcherId?: string
  active: boolean
  panes: number
  user?: string
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
  currentPath?: string
  startCommand?: string
  currentCommand?: string
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

export type OpsServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable'

export type OpsServiceStatus = {
  name: string
  displayName: string
  trackingMode: 'builtin' | 'custom'
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
  condition: {
    activeState?: string
    subState?: string
    result?: string
    exitCode?: number
    exitStatus?: number
    transitionedAt?: string
  }
  properties?: Record<string, string>
  output?: string
  observedAt: string
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
  targetService?: string
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

export type OpsRunbookExecutionSnapshot = {
  schemaVersion: number
  runbookId: string
  name: string
  description: string
  steps: Array<OpsRunbookStep>
  parameters: Array<RunbookParameter>
  webhookURL: string
  targetKind?: 'service'
  targetName?: string
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
  source?: 'runbooks' | 'scheduler' | 'now'
  targetKind?: 'service'
  targetName?: string
  definition?: OpsRunbookExecutionSnapshot
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
  globalRev?: number
}

export type OpsServiceActionResponse = {
  service: OpsServiceStatus
  services: Array<OpsServiceStatus>
  overview: OpsOverview
  globalRev: number
  verification: OpsServiceActionVerification
}

export type OpsServiceStatusResponse = {
  status: OpsServiceInspect
  context: {
    runbook: OpsRunbook | null
    latestRun: OpsRunbookRun | null
  }
}

export type OpsServiceLogsResponse = {
  service: string
  lines: number
  output: string
  since?: string
}

export type OpsServiceActionVerification = {
  state: 'confirmed' | 'mismatch' | 'unavailable'
  field: 'activeState' | 'enabledState'
  expected: string
  observed: string
  observedAt: string
  attempts: number
}

export type OpsHostMetrics = {
  cpuPercent: number
  cpuCount: number
  loadAvg1: number
  loadAvg5: number
  loadAvg15: number
  loadPerCPU: number
  memUsedBytes: number
  memTotalBytes: number
  memAvailableBytes: number
  memPercent: number
  swapUsedBytes: number
  swapTotalBytes: number
  swapPercent: number
  diskUsedBytes: number
  diskTotalBytes: number
  diskFreeBytes: number
  diskPercent: number
  diskInodesUsed: number
  diskInodesTotal: number
  diskInodesPercent: number
  netRxBytes: number
  netTxBytes: number
  netInterfaces: number
  processCount: number
  threadCount: number
  hostUptimeSec: number
  bootTime: string
  cpuPressureAvg10: number
  memPressureAvg10: number
  ioPressureAvg10: number
  numGoroutines: number
  goMemAllocMB: number
  goMemSysMB: number
  goHeapObjects: number
  goNumGC: number
  goLastGcPauseMs: number
  collectedAt: string
}

export type MetricPostureSignal = {
  name:
    | 'cpu'
    | 'memory'
    | 'rootDisk'
    | 'inodes'
    | 'swap'
    | 'cpuPressure'
    | 'memoryPressure'
    | 'ioPressure'
  severity: 'warning' | 'critical'
  value: number
  since: string
}

export type MetricPosture = {
  state: 'normal' | 'pressure' | 'unavailable'
  severity: 'ok' | 'warning' | 'critical' | 'unknown'
  warningCount: number
  criticalCount: number
  signals: Array<MetricPostureSignal>
  observedAt: string
}

export type OpsMetricsResponse = {
  metrics: OpsHostMetrics
  posture: MetricPosture
}

export type NowSourceStatus = 'current' | 'stale' | 'unavailable' | 'not_configured'

export type NowSource = {
  status: NowSourceStatus
  checkedAt: string
  message?: string
}

export type NowSources = {
  tmux: NowSource
  services: NowSource
  metrics: NowSource
  runbooks: NowSource
}

export type NowRunbookReference = {
  id: string
  name: string
  description?: string
  parameters: Array<RunbookParameter>
  targetService?: string
  steps: Array<OpsRunbookStep>
}

export type NowRunReference = {
  runbookId: string
  runbookName: string
  runId: string
  status: string
  source?: 'runbooks' | 'scheduler' | 'now'
  targetKind?: 'service'
  targetName?: string
  createdAt: string
}

export type NowServiceReference = {
  name: string
  displayName: string
  trackingMode: 'builtin' | 'custom'
  manager: string
  scope: string
  unit: string
}

export type NowRunbookApprovalAttention = {
  type: 'runbook_approval'
  run: NowRunReference
}

export type NowServiceFailedAttention = {
  type: 'service_failed'
  service: NowServiceReference
  runbook?: NowRunbookReference
  failure?: NowRunReference
}

export type NowRunbookFailedAttention = {
  type: 'runbook_failed'
  run: NowRunReference
}

export type NowMetricsPressureAttention = {
  type: 'metrics_pressure'
  severity: 'warning' | 'critical'
  signals: Array<MetricPostureSignal>
}

export type NowAttentionItem =
  | NowRunbookApprovalAttention
  | NowServiceFailedAttention
  | NowRunbookFailedAttention
  | NowMetricsPressureAttention

export type NowAttention = {
  total: number
  visible: Array<NowAttentionItem>
  overflow: {
    approvals: number
    services: number
    runbooks: number
    metrics: number
  }
}

export type NowInProgressRun = {
  id: string
  runbookId: string
  runbookName: string
  status: 'queued' | 'running'
  totalSteps: number
  completedSteps: number
  currentStep?: string
  source?: 'runbooks' | 'scheduler' | 'now'
  targetKind?: 'service'
  targetName?: string
  createdAt: string
  startedAt?: string
}

export type NowInProgressSession = {
  name: string
  user?: string
  pinned: boolean
  unreadWindows: number
  unreadPanes: number
  activityAt: string
}

export type NowSnapshot = {
  generatedAt: string
  reliability: {
    state: 'normal' | 'attention' | 'degraded'
    services: {
      tracked: number
      running: number
      failed: number
      inactive: number
      unknown: number
    }
    metrics: MetricPosture
  }
  attention: NowAttention
  inProgress: {
    runs: Array<NowInProgressRun>
    sessions: Array<NowInProgressSession>
  }
  sources: NowSources
}

export type NowResponse = {
  now: NowSnapshot
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
  trackingMode?: 'builtin' | 'custom'
}

export type OpsBrowseServicesResponse = {
  services: Array<OpsBrowsedService>
}

export type OpsUnitActionResponse = {
  service: OpsServiceStatus
  overview: OpsOverview
  globalRev: number
  verification: OpsServiceActionVerification
}

export type OpsUnitLogsResponse = {
  unit: string
  lines: number
  output: string
  since?: string
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
  | { type: 'ops.metrics.updated'; payload: OpsMetricsResponse }
  | { type: 'ops.posture.updated'; payload: { posture: MetricPosture } }
  | { type: 'ops.job.updated'; payload: { job: OpsRunbookRun } }
  | { type: 'ops.schedule.updated'; payload: Record<string, unknown> }
