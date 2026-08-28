(() => {
  'use strict'

  const nativeFetch = window.fetch.bind(window)
  const NativeWebSocket = window.WebSocket
  const observedAt = '2026-07-27T18:00:00Z'
  const pressureSince = '2026-07-27T17:42:00Z'
  const settingsRevision = '7'.repeat(64)

  const service = (
    name,
    displayName,
    unit,
    activeState,
    enabledState = 'enabled',
  ) => ({
    name,
    displayName,
    trackingMode: 'custom',
    manager: 'systemd',
    scope: 'system',
    unit,
    exists: true,
    enabledState,
    activeState,
    updatedAt: observedAt,
  })

  const telemetryService = service(
    'telemetry-relay',
    'Telemetry Relay',
    'telemetry-relay.service',
    'failed',
  )
  const navigationService = service(
    'navigation-core',
    'Navigation Core',
    'navigation-core.service',
    'active',
  )
  const payloadService = service(
    'payload-link',
    'Payload Link',
    'payload-link.service',
    'active',
  )
  const services = [telemetryService, navigationService, payloadService]

  const recoverySteps = [
    {
      type: 'run',
      title: 'Inspect relay condition',
      command: 'stationctl telemetry inspect',
      timeout: 30,
    },
    {
      type: 'approval',
      title: 'Authorize antenna failover',
      description: 'Confirm the backup antenna path before switching telemetry.',
    },
    {
      type: 'run',
      title: 'Verify downlink',
      command: 'stationctl telemetry verify',
      timeout: 45,
      retries: 2,
      retryDelay: 2,
    },
  ]

  const recoveryRunbook = {
    id: 'recover-telemetry-relay',
    name: 'Recover telemetry relay',
    description: 'Restore the downlink and verify the service against current evidence.',
    enabled: true,
    targetService: 'telemetry-relay',
    parameters: [
      {
        name: 'antenna',
        label: 'Backup antenna',
        type: 'select',
        default: 'antenna-b',
        required: true,
        options: ['antenna-b', 'antenna-c'],
      },
    ],
    steps: recoverySteps,
    createdAt: '2026-07-20T09:00:00Z',
    updatedAt: '2026-07-26T16:30:00Z',
  }

  const completedJob = {
    id: 'job-orbital-042',
    runbookId: recoveryRunbook.id,
    runbookName: recoveryRunbook.name,
    status: 'succeeded',
    totalSteps: 3,
    completedSteps: 3,
    currentStep: '',
    error: '',
    source: 'now',
    targetKind: 'service',
    targetName: 'telemetry-relay',
    parametersUsed: { antenna: 'antenna-b' },
    definition: {
      schemaVersion: 1,
      runbookId: recoveryRunbook.id,
      name: recoveryRunbook.name,
      description: recoveryRunbook.description,
      steps: recoverySteps,
      parameters: recoveryRunbook.parameters,
      webhookURL: '',
      targetKind: 'service',
      targetName: 'telemetry-relay',
    },
    stepResults: [
      {
        stepIndex: 0,
        title: 'Inspect relay condition',
        type: 'run',
        output: 'Downlink carrier absent; backup path is ready.',
        error: '',
        durationMs: 824,
      },
      {
        stepIndex: 1,
        title: 'Authorize antenna failover',
        type: 'approval',
        output: 'Approved by operator for antenna-b.',
        error: '',
        durationMs: 1260,
      },
      {
        stepIndex: 2,
        title: 'Verify downlink',
        type: 'run',
        output: 'Telemetry frames stable; verification passed.',
        error: '',
        durationMs: 1840,
      },
    ],
    createdAt: '2026-07-27T17:51:00Z',
    startedAt: '2026-07-27T17:51:02Z',
    finishedAt: '2026-07-27T17:53:18Z',
  }

  const approvalJob = {
    id: 'job-orbital-043',
    runbookId: recoveryRunbook.id,
    runbookName: recoveryRunbook.name,
    status: 'waiting_approval',
    totalSteps: 3,
    completedSteps: 1,
    currentStep: 'Authorize antenna failover',
    error: '',
    source: 'now',
    targetKind: 'service',
    targetName: 'telemetry-relay',
    stepResults: [
      {
        stepIndex: 0,
        title: 'Inspect relay condition',
        type: 'run',
        output: 'Primary path degraded; backup path available.',
        error: '',
        durationMs: 690,
      },
    ],
    createdAt: '2026-07-27T17:58:00Z',
    startedAt: '2026-07-27T17:58:02Z',
  }

  const posture = {
    state: 'pressure',
    severity: 'critical',
    warningCount: 1,
    criticalCount: 1,
    signals: [
      {
        name: 'cpuPressure',
        severity: 'critical',
        value: 31.4,
        since: pressureSince,
      },
      {
        name: 'ioPressure',
        severity: 'warning',
        value: 14.7,
        since: pressureSince,
      },
    ],
    observedAt,
  }

  const normalPosture = {
    state: 'normal',
    severity: 'ok',
    warningCount: 0,
    criticalCount: 0,
    signals: [],
    observedAt,
  }

  function metricsSample(index = 0) {
    const collectedAt = new Date(Date.parse(observedAt) + index * 15_000).toISOString()
    return {
      cpuPercent: 68 + (index % 4) * 2.3,
      cpuCount: 12,
      loadAvg1: 7.6 + index * 0.08,
      loadAvg5: 6.8,
      loadAvg15: 5.9,
      loadPerCPU: 0.64 + index * 0.006,
      memUsedBytes: 20_401_094_656 + index * 12_582_912,
      memTotalBytes: 34_359_738_368,
      memAvailableBytes: 13_958_643_712 - index * 12_582_912,
      memPercent: 59.4 + index * 0.04,
      swapUsedBytes: 536_870_912,
      swapTotalBytes: 4_294_967_296,
      swapPercent: 12.5,
      diskUsedBytes: 322_122_547_200,
      diskTotalBytes: 536_870_912_000,
      diskFreeBytes: 214_748_364_800,
      diskPercent: 60,
      diskInodesUsed: 2_840_000,
      diskInodesTotal: 8_000_000,
      diskInodesPercent: 35.5,
      netRxBytes: 8_400_000_000 + index * 4_800_000,
      netTxBytes: 3_100_000_000 + index * 2_200_000,
      netInterfaces: 2,
      processCount: 186 + (index % 3),
      threadCount: 1240 + index * 2,
      hostUptimeSec: 1_442_600 + index * 15,
      bootTime: '2026-07-11T01:16:40Z',
      cpuPressureAvg10: 25.2 + index * 0.7,
      memPressureAvg10: 3.1 + index * 0.08,
      ioPressureAvg10: 11.8 + index * 0.31,
      sensors: {
        temperatures: [
          {
            id: 'hwmon0:temp1',
            label: 'CPU package',
            source: 'coretemp',
            celsius: 68.4 + index * 0.15,
            maxCelsius: 85,
            criticalCelsius: 100,
            alarm: false,
          },
          {
            id: 'hwmon1:temp1',
            label: 'NVMe composite',
            source: 'nvme',
            celsius: 54.2 + index * 0.08,
            criticalCelsius: 84.8,
          },
        ],
        fans: [
          {
            id: 'hwmon2:fan1',
            label: 'Chassis fan',
            source: 'nct6798',
            rpm: 1280 + index * 9,
            minRpm: 500,
            alarm: false,
          },
        ],
        power: [
          {
            id: 'powercap:intel-rapl:0',
            label: 'Package power',
            source: 'powercap',
            watts: 31.6 + index * 0.4,
            capWatts: 65,
          },
        ],
      },
      numGoroutines: 48 + (index % 4),
      goMemAllocMB: 92.4 + index * 0.6,
      goMemSysMB: 138.2,
      goHeapObjects: 84_120 + index * 64,
      goNumGC: 217 + index,
      goLastGcPauseMs: 0.42 + index * 0.01,
      collectedAt,
    }
  }

  const overview = {
    host: {
      hostname: 'orbital-station',
      os: 'linux',
      arch: 'amd64',
      cpus: 12,
      goVersion: 'go1.26',
    },
    sentinel: {
      pid: 4242,
      uptimeSec: 86_400,
    },
    services: {
      total: 3,
      active: 2,
      failed: 1,
    },
    updatedAt: observedAt,
  }

  function source(status = 'current') {
    return { status, observedAt }
  }

  function stringSetting(
    effectiveValue,
    defaultValue,
    applyMode,
    validation = {},
  ) {
    return {
      persistedValue: effectiveValue,
      effectiveValue,
      defaultValue,
      source: 'file',
      editable: true,
      applyMode,
      restartPending: false,
      validation: {
        required: true,
        allowCustom: true,
        options: [],
        ...validation,
      },
    }
  }

  function booleanSetting(effectiveValue, defaultValue = false) {
    return {
      persistedValue: effectiveValue,
      effectiveValue,
      defaultValue,
      source: 'file',
      editable: true,
      applyMode: 'restart',
      restartPending: false,
      validation: { required: true },
    }
  }

  function integerSetting(effectiveValue, defaultValue, min, max) {
    return {
      persistedValue: effectiveValue,
      effectiveValue,
      defaultValue,
      source: 'file',
      editable: true,
      applyMode: 'restart',
      restartPending: false,
      validation: { required: true, min, max, step: 1 },
    }
  }

  function listSetting(effectiveValue, options = [], allowCustom = true) {
    return {
      persistedValue: effectiveValue,
      effectiveValue,
      defaultValue: [],
      source: 'file',
      editable: true,
      applyMode: 'restart',
      restartPending: false,
      validation: {
        required: false,
        allowCustom,
        options,
      },
    }
  }

  function sensitiveSetting(configured) {
    return {
      configured,
      source: 'file',
      editable: true,
      applyMode: 'restart',
      restartPending: false,
      validation: { required: false },
    }
  }

  function settingsSnapshot() {
    const configPath = '/opt/orbital-station/sentinel.toml'
    const backupPath = `${configPath}.bak`
    const accountOptions = [
      { value: 'flight-operator', label: 'flight-operator' },
      { value: 'payload-operator', label: 'payload-operator' },
      { value: 'root', label: 'root' },
    ]
    return {
      revision: settingsRevision,
      metadata: { version: 'showcase' },
      deployment: {
        scope: 'standalone',
        runtimeMode: 'standalone',
        configPath,
      },
      restart: {
        required: false,
        changedKeys: [],
        backupPath,
        instruction:
          'Restart Sentinel with the external supervisor that owns this process.',
      },
      experience: {
        timezone: stringSetting('UTC', 'Local', 'partial', {
          format: 'iana-timezone',
          options: [
            { value: 'Local', label: 'Local' },
            { value: 'UTC', label: 'UTC' },
            { value: 'America/Los_Angeles', label: 'America/Los_Angeles' },
          ],
        }),
        locale: stringSetting('en-US', '', 'live', {
          required: false,
          allowCustom: false,
          format: 'bcp-47',
          options: [
            { value: '', label: 'Browser default' },
            { value: 'en-US', label: 'English (US)' },
            { value: 'pt-BR', label: 'Português (Brasil)' },
          ],
        }),
      },
      operations: {
        watchtower: {
          enabled: booleanSetting(true, true),
          tickInterval: stringSetting('2s', '1s', 'restart', {
            format: 'duration',
            min: '100ms',
            max: '1m',
          }),
          captureLines: integerSetting(120, 80, 1, 2000),
          captureTimeout: stringSetting('250ms', '150ms', 'restart', {
            format: 'duration',
            min: '10ms',
            max: '10s',
          }),
          journalRows: integerSetting(8000, 5000, 100, 1_000_000),
        },
        runbooks: {
          maxConcurrent: integerSetting(4, 5, 1, 64),
        },
        log: {
          level: stringSetting('info', 'info', 'restart', {
            allowCustom: false,
            options: [
              { value: 'debug', label: 'Debug' },
              { value: 'info', label: 'Info' },
              { value: 'warn', label: 'Warn' },
              { value: 'error', label: 'Error' },
            ],
          }),
        },
      },
      integrations: {
        mcp: {
          enabled: booleanSetting(true, false),
          token: sensitiveSetting(true),
          runtimeTokenConfigured: true,
          endpoint: '/mcp',
        },
        healthReport: {
          schedule: stringSetting('0 9 * * 1-5', '', 'restart', {
            required: false,
            format: 'cron',
          }),
          webhookUrl: sensitiveSetting(true),
          nextActivation: '2026-07-28T09:00:00Z',
        },
      },
      accounts: {
        processUser: 'flight-operator',
        processIsRoot: false,
        inventoryAvailable: true,
        users: [
          {
            name: 'flight-operator',
            processUser: true,
            root: false,
            allowed: true,
          },
          {
            name: 'payload-operator',
            processUser: false,
            root: false,
            allowed: true,
          },
          { name: 'root', processUser: false, root: true, allowed: false },
        ],
        allowedUsers: listSetting(
          ['flight-operator', 'payload-operator'],
          accountOptions,
          false,
        ),
        allowRootTarget: booleanSetting(false, false),
        userSwitchMethod: stringSetting(
          'systemd-run',
          'systemd-run',
          'restart',
          {
            allowCustom: false,
            options: [
              { value: 'sudo', label: 'sudo' },
              { value: 'systemd-run', label: 'systemd-run' },
            ],
          },
        ),
        methodCapabilities: [
          {
            value: 'sudo',
            label: 'sudo',
            available: true,
            detail:
              'sudo is installed; passwordless policy must still be configured',
          },
          {
            value: 'systemd-run',
            label: 'systemd-run',
            available: true,
            detail:
              'systemd-run is installed; policy must still be configured outside Sentinel',
          },
        ],
        privilegeGuidance:
          'Sentinel detects executables but cannot grant host privileges.',
      },
      access: {
        listener: {
          host: stringSetting('192.0.2.44', '127.0.0.1', 'restart', {
            format: 'listen-host',
          }),
          port: integerSetting(4040, 4040, 1, 65535),
          classification: 'specific',
          address: '192.0.2.44:4040',
        },
        authentication: {
          token: sensitiveSetting(true),
          runtimeTokenConfigured: true,
        },
        origins: {
          allowed: listSetting(['https://control.orbital.example']),
        },
        proxies: {
          trusted: listSetting(['192.0.2.0/24']),
        },
        cookies: {
          secure: stringSetting('always', 'auto', 'restart', {
            allowCustom: false,
            options: [
              { value: 'auto', label: 'Auto' },
              { value: 'always', label: 'Always secure' },
              { value: 'never', label: 'Never secure' },
            ],
          }),
          allowInsecure: booleanSetting(false, false),
        },
        recovery: {
          configPath,
          backupPath,
          restoreCommand: `cp -- '${backupPath}' '${configPath}'`,
          validateCommand: `sentinel --config '${configPath}' config validate --effective`,
          instruction:
            'Restore the adjacent backup, validate it, and restart the same deployment manually.',
        },
      },
      diagnostics: {
        configExists: true,
        environmentOwnedKeys: [],
        readOnlyKeys: ['version', 'storage.path', 'log.path'],
        deploymentDetection: 'standalone',
      },
    }
  }

  function nowSnapshot() {
    const healthy = localStorage.getItem('sentinel_docs_showcase_scene') === 'healthy'
    return {
      generatedAt: observedAt,
      confidence: {
        state: 'current',
        sources: {
          tmux: source(),
          services: source(),
          metrics: source(),
          runbooks: source(),
        },
      },
      posture: {
        state: healthy ? 'healthy' : 'at_risk',
        services: healthy
          ? { tracked: 3, running: 3, failed: 0, inactive: 0, unknown: 0 }
          : { tracked: 3, running: 2, failed: 1, inactive: 0, unknown: 0 },
        metrics: healthy ? normalPosture : posture,
      },
      attention: healthy
        ? {
            total: 0,
            visible: [],
            overflow: { approvals: 0, services: 0, metrics: 0 },
          }
        : {
            total: 3,
            visible: [
              {
                type: 'runbook_approval',
                run: {
                  runbookId: recoveryRunbook.id,
                  runbookName: recoveryRunbook.name,
                  runId: approvalJob.id,
                  status: approvalJob.status,
                  source: 'now',
                  targetKind: 'service',
                  targetName: 'telemetry-relay',
                  createdAt: approvalJob.createdAt,
                },
              },
              {
                type: 'service_failed',
                service: {
                  name: telemetryService.name,
                  displayName: telemetryService.displayName,
                  trackingMode: telemetryService.trackingMode,
                  manager: telemetryService.manager,
                  scope: telemetryService.scope,
                  unit: telemetryService.unit,
                },
                runbook: recoveryRunbook,
              },
              {
                type: 'metrics_pressure',
                severity: 'critical',
                signals: posture.signals,
                observedAt,
              },
            ],
            overflow: { approvals: 0, services: 0, metrics: 0 },
          },
      inProgress: healthy
        ? { runs: [], sessions: [] }
        : {
            runs: [
              {
                id: approvalJob.id,
                runbookId: approvalJob.runbookId,
                runbookName: approvalJob.runbookName,
                status: 'running',
                totalSteps: 3,
                completedSteps: 1,
                currentStep: approvalJob.currentStep,
                source: 'now',
                targetKind: 'service',
                targetName: 'telemetry-relay',
                createdAt: approvalJob.createdAt,
                startedAt: approvalJob.startedAt,
              },
            ],
            sessions: [
              {
                name: 'flight-control',
                user: 'operator',
                pinned: true,
                unreadWindows: 1,
                unreadPanes: 2,
                activityAt: observedAt,
              },
              {
                name: 'telemetry',
                user: 'operator',
                pinned: false,
                unreadWindows: 1,
                unreadPanes: 1,
                activityAt: observedAt,
              },
            ],
          },
    }
  }

  function statusResponse() {
    const receiptView = window.location.pathname === '/runbooks'
    const activeService = { ...telemetryService, activeState: 'active' }
    return {
      status: {
        service: receiptView ? activeService : telemetryService,
        summary: receiptView
          ? 'Recovered and verified after operator approval'
          : 'Downlink relay stopped after carrier loss',
        condition: receiptView
          ? {
              activeState: 'active',
              subState: 'running',
              result: 'success',
              exitCode: 0,
              exitStatus: 0,
              transitionedAt: '2026-07-27T17:53:16Z',
            }
          : {
              activeState: 'failed',
              subState: 'failed',
              result: 'exit-code',
              exitCode: 1,
              exitStatus: 23,
              transitionedAt: '2026-07-27T17:42:00Z',
            },
        properties: {
          Description: 'Orbital telemetry downlink relay',
          Mission: 'Orbital Station',
          RecoveryPolicy: 'operator-approved',
        },
        output: receiptView
          ? 'Downlink synchronized\nTelemetry frames stable\nVerification passed'
          : 'Carrier lock lost\nPrimary antenna path unavailable\nAwaiting recovery procedure',
        observedAt,
      },
      context: {
        runbook: recoveryRunbook,
        latestRun: completedJob,
      },
    }
  }

  function responseData(pathname) {
    if (pathname === '/api/meta') {
      return {
        tokenRequired: false,
        defaultCwd: '/tmp',
        version: 'showcase',
        timezone: 'UTC',
        locale: 'en-US',
        hostname: 'orbital-station',
        processUser: 'operator',
        userSwitchMethod: '',
        isRoot: false,
        canSwitchUser: false,
        allowedUsers: [],
      }
    }
    if (pathname === '/api/ops/settings') return settingsSnapshot()
    if (pathname === '/api/now') return { now: nowSnapshot() }
    if (pathname === '/api/ops/overview') return { overview }
    if (pathname === '/api/ops/metrics') return { metrics: metricsSample(), posture }
    if (pathname === '/api/ops/services') return { services }
    if (pathname === '/api/ops/services/browse') {
      return {
        services: services.map((item) => ({
          unit: item.unit,
          unitType: 'service',
          description: item.displayName,
          activeState: item.activeState,
          enabledState: item.enabledState,
          manager: item.manager,
          scope: item.scope,
          tracked: true,
          trackedName: item.name,
          trackingMode: item.trackingMode,
        })),
      }
    }
    if (pathname === '/api/ops/services/telemetry-relay/status') return statusResponse()
    if (pathname === '/api/ops/services/telemetry-relay/logs') {
      return {
        service: 'telemetry-relay',
        lines: 5,
        since: '2026-07-27T17:42:00Z',
        output:
          '17:42:00 carrier lock lost\n17:42:01 relay entered failed state\n17:51:02 recovery started from Now\n17:52:20 antenna-b approved\n17:53:18 downlink verification passed',
      }
    }
    if (pathname === '/api/ops/runbooks') {
      return {
        runbooks: [recoveryRunbook],
        jobs: [approvalJob, completedJob],
        schedules: [],
      }
    }
    if (pathname === '/api/ops/jobs/job-orbital-042') return { job: completedJob }
    if (pathname === '/api/ops/jobs/job-orbital-043') return { job: approvalJob }
    return null
  }

  window.fetch = async (input, init) => {
    const request = input instanceof Request ? input : null
    const method = String(init?.method || request?.method || 'GET').toUpperCase()
    const url = new URL(request?.url || String(input), window.location.origin)
    if (method === 'GET') {
      const data = responseData(url.pathname)
      if (data !== null) {
        const headers = {
          'Content-Type': 'application/json',
          'X-Sentinel-Showcase': 'orbital-station',
        }
        if (url.pathname === '/api/ops/settings') {
          headers.ETag = `"${settingsRevision}"`
        }
        return new Response(JSON.stringify({ data }), {
          status: 200,
          headers,
        })
      }
    }
    return nativeFetch(input, init)
  }

  class ShowcaseEventsSocket extends EventTarget {
    static CONNECTING = 0
    static OPEN = 1
    static CLOSING = 2
    static CLOSED = 3

    CONNECTING = 0
    OPEN = 1
    CLOSING = 2
    CLOSED = 3
    binaryType = 'blob'
    bufferedAmount = 0
    extensions = ''
    protocol = ''
    readyState = ShowcaseEventsSocket.CONNECTING
    onopen = null
    onmessage = null
    onerror = null
    onclose = null

    constructor(url) {
      super()
      this.url = String(url)
      this.timers = []
      const openTimer = window.setTimeout(() => {
        if (this.readyState !== ShowcaseEventsSocket.CONNECTING) return
        this.readyState = ShowcaseEventsSocket.OPEN
        this.emit('open', new Event('open'))
        for (let index = 1; index <= 12; index += 1) {
          const timer = window.setTimeout(() => {
            if (this.readyState !== ShowcaseEventsSocket.OPEN) return
            const event = new MessageEvent('message', {
              data: JSON.stringify({
                type: 'ops.metrics.updated',
                payload: { metrics: metricsSample(index), posture },
              }),
            })
            this.emit('message', event)
          }, index * 120)
          this.timers.push(timer)
        }
      }, 30)
      this.timers.push(openTimer)
    }

    emit(type, event) {
      this.dispatchEvent(event)
      const handler = this[`on${type}`]
      if (typeof handler === 'function') handler.call(this, event)
    }

    send() {}

    close(code = 1000, reason = '') {
      if (this.readyState === ShowcaseEventsSocket.CLOSED) return
      this.readyState = ShowcaseEventsSocket.CLOSING
      for (const timer of this.timers) window.clearTimeout(timer)
      this.timers = []
      this.readyState = ShowcaseEventsSocket.CLOSED
      this.emit('close', new CloseEvent('close', { code, reason, wasClean: true }))
    }
  }

  function ShowcaseWebSocket(url, protocols) {
    const parsed = new URL(String(url), window.location.origin)
    if (parsed.pathname === '/ws/events') return new ShowcaseEventsSocket(parsed.toString())
    return new NativeWebSocket(url, protocols)
  }

  Object.defineProperties(ShowcaseWebSocket, {
    CONNECTING: { value: NativeWebSocket.CONNECTING },
    OPEN: { value: NativeWebSocket.OPEN },
    CLOSING: { value: NativeWebSocket.CLOSING },
    CLOSED: { value: NativeWebSocket.CLOSED },
  })
  ShowcaseWebSocket.prototype = NativeWebSocket.prototype
  window.WebSocket = ShowcaseWebSocket
})()
