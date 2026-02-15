import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  Bell,
  Clock3,
  Menu,
  Play,
  RefreshCw,
  RotateCw,
  Square,
} from 'lucide-react'
import type {
  ConnectionState,
  OpsAlert,
  OpsAlertsResponse,
  OpsOverview,
  OpsOverviewResponse,
  OpsRunbook,
  OpsRunbookRun,
  OpsRunbookRunResponse,
  OpsRunbooksResponse,
  OpsServiceAction,
  OpsServiceActionResponse,
  OpsServiceStatus,
  OpsServicesResponse,
  OpsTimelineEvent,
  OpsTimelineResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import OpsSidebar from '@/components/OpsSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'
import { buildWSProtocols } from '@/lib/wsAuth'
import { cn } from '@/lib/utils'

function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.trunc(totalSeconds))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

function serviceRowTone(service: OpsServiceStatus): string {
  const state = service.activeState.trim().toLowerCase()
  if (state === 'active' || state === 'running') {
    return 'border-emerald-500/40 bg-emerald-500/10'
  }
  if (state === 'failed') {
    return 'border-red-500/40 bg-red-500/10'
  }
  return 'border-border-subtle bg-surface-elevated'
}

function opsTabButtonClass(active: boolean): string {
  return cn(
    'inline-flex cursor-pointer items-center gap-1 rounded-md border px-2.5 py-1 text-[11px] font-medium transition-colors',
    active
      ? 'border-primary/40 bg-primary/15 text-primary-text-bright'
      : 'border-transparent text-muted-foreground hover:border-border hover:bg-surface-overlay hover:text-foreground',
  )
}

function OpsPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)

  const [overview, setOverview] = useState<OpsOverview | null>(null)
  const [services, setServices] = useState<Array<OpsServiceStatus>>([])
  const [alerts, setAlerts] = useState<Array<OpsAlert>>([])
  const [timelineEvents, setTimelineEvents] = useState<Array<OpsTimelineEvent>>(
    [],
  )
  const [runbooks, setRunbooks] = useState<Array<OpsRunbook>>([])
  const [jobs, setJobs] = useState<Array<OpsRunbookRun>>([])
  const [opsTab, setOpsTab] = useState<
    'services' | 'alerts' | 'timeline' | 'runbooks'
  >('services')
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [servicesLoading, setServicesLoading] = useState(false)
  const [alertsLoading, setAlertsLoading] = useState(false)
  const [timelineLoading, setTimelineLoading] = useState(false)
  const [runbooksLoading, setRunbooksLoading] = useState(false)
  const [overviewError, setOverviewError] = useState('')
  const [servicesError, setServicesError] = useState('')
  const [alertsError, setAlertsError] = useState('')
  const [timelineError, setTimelineError] = useState('')
  const [runbooksError, setRunbooksError] = useState('')
  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('connecting')
  const [pendingActions, setPendingActions] = useState<
    Partial<Record<string, OpsServiceAction>>
  >({})

  const previousServiceRef = useRef(new Map<string, OpsServiceStatus>())

  const refreshOverview = useCallback(
    async (background = false) => {
      if (!background) setOverviewLoading(true)
      try {
        const data = await api<OpsOverviewResponse>('/api/ops/overview')
        setOverview(data.overview)
        setOverviewError('')
      } catch (error) {
        setOverviewError(
          error instanceof Error ? error.message : 'failed to load overview',
        )
      } finally {
        if (!background) setOverviewLoading(false)
      }
    },
    [api],
  )

  const refreshServices = useCallback(
    async (background = false) => {
      if (!background) setServicesLoading(true)
      try {
        const data = await api<OpsServicesResponse>('/api/ops/services')
        setServices(data.services)
        setServicesError('')
      } catch (error) {
        setServicesError(
          error instanceof Error ? error.message : 'failed to load services',
        )
      } finally {
        if (!background) setServicesLoading(false)
      }
    },
    [api],
  )

  const refreshAlerts = useCallback(
    async (background = false) => {
      if (!background) setAlertsLoading(true)
      try {
        const data = await api<OpsAlertsResponse>('/api/ops/alerts?limit=100')
        setAlerts(data.alerts)
        setAlertsError('')
      } catch (error) {
        setAlertsError(
          error instanceof Error ? error.message : 'failed to load alerts',
        )
      } finally {
        if (!background) setAlertsLoading(false)
      }
    },
    [api],
  )

  const refreshTimeline = useCallback(
    async (background = false) => {
      if (!background) setTimelineLoading(true)
      try {
        const params = new URLSearchParams({ limit: '200' })
        if (timelineQuery.trim() !== '') params.set('q', timelineQuery.trim())
        if (timelineSeverity !== 'all') params.set('severity', timelineSeverity)
        const data = await api<OpsTimelineResponse>(
          `/api/ops/timeline?${params.toString()}`,
        )
        setTimelineEvents(data.events)
        setTimelineError('')
      } catch (error) {
        setTimelineError(
          error instanceof Error ? error.message : 'failed to load timeline',
        )
      } finally {
        if (!background) setTimelineLoading(false)
      }
    },
    [api, timelineQuery, timelineSeverity],
  )

  const refreshRunbooks = useCallback(
    async (background = false) => {
      if (!background) setRunbooksLoading(true)
      try {
        const data = await api<OpsRunbooksResponse>('/api/ops/runbooks')
        setRunbooks(data.runbooks)
        setJobs(data.jobs)
        setRunbooksError('')
      } catch (error) {
        setRunbooksError(
          error instanceof Error ? error.message : 'failed to load runbooks',
        )
      } finally {
        if (!background) setRunbooksLoading(false)
      }
    },
    [api],
  )

  const refreshAll = useCallback(() => {
    void refreshOverview()
    void refreshServices()
    void refreshAlerts()
    void refreshRunbooks()
  }, [refreshAlerts, refreshOverview, refreshRunbooks, refreshServices])

  const refreshPage = useCallback(() => {
    refreshAll()
    void refreshTimeline()
  }, [refreshAll, refreshTimeline])

  useEffect(() => {
    refreshAll()
  }, [refreshAll])

  useEffect(() => {
    void refreshTimeline(true)
  }, [refreshTimeline])

  useEffect(() => {
    if (tokenRequired && token.trim() === '') {
      setConnectionState('disconnected')
      return
    }

    let disposed = false
    let socket: WebSocket | null = null
    let retryTimer: ReturnType<typeof setTimeout> | null = null

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
        const typed = message as {
          type?: string
          payload?: {
            services?: Array<OpsServiceStatus>
            overview?: OpsOverview
            alerts?: Array<OpsAlert>
            event?: OpsTimelineEvent
            job?: OpsRunbookRun
          }
        }
        switch (typed.type) {
          case 'ops.services.updated':
            if (Array.isArray(typed.payload?.services)) {
              setServices(typed.payload.services)
              setServicesError('')
            } else {
              void refreshServices(true)
            }
            break
          case 'ops.overview.updated':
            if (
              typed.payload?.overview != null &&
              typeof typed.payload.overview === 'object'
            ) {
              setOverview(typed.payload.overview)
              setOverviewError('')
            } else {
              void refreshOverview(true)
            }
            break
          case 'ops.alerts.updated':
            if (Array.isArray(typed.payload?.alerts)) {
              setAlerts(typed.payload.alerts)
              setAlertsError('')
            } else {
              void refreshAlerts(true)
            }
            break
          case 'ops.timeline.updated':
            if (typed.payload?.event != null) {
              const timelineEvent = typed.payload.event
              setTimelineEvents((prev) => [
                timelineEvent,
                ...prev.filter((item) => item.id !== timelineEvent.id),
              ])
              setTimelineError('')
            } else {
              void refreshTimeline(true)
            }
            break
          case 'ops.job.updated':
            if (typed.payload?.job != null) {
              const job = typed.payload.job
              setJobs((prev) => {
                const next = prev.filter((item) => item.id !== job.id)
                return [job, ...next]
              })
            } else {
              void refreshRunbooks(true)
            }
            break
          default:
            break
        }
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
        retryTimer = window.setTimeout(connect, 1_200)
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
  }, [
    refreshAlerts,
    refreshOverview,
    refreshServices,
    refreshTimeline,
    refreshRunbooks,
    token,
    tokenRequired,
  ])

  const runServiceAction = useCallback(
    async (serviceName: string, action: OpsServiceAction) => {
      const previous = services.find((item) => item.name === serviceName)
      if (!previous) return

      previousServiceRef.current.set(serviceName, previous)
      setPendingActions((prev) => ({ ...prev, [serviceName]: action }))
      setServices((prev) =>
        prev.map((item) =>
          item.name === serviceName
            ? withOptimisticServiceAction(item, action)
            : item,
        ),
      )

      try {
        const data = await api<OpsServiceActionResponse>(
          `/api/ops/services/${encodeURIComponent(serviceName)}/action`,
          {
            method: 'POST',
            body: JSON.stringify({ action }),
          },
        )
        if (Array.isArray(data.services) && data.services.length > 0) {
          setServices(data.services)
        } else {
          setServices((prev) => upsertOpsService(prev, data.service))
        }
        setOverview(data.overview)
        if (Array.isArray(data.alerts)) {
          setAlerts(data.alerts)
        }
        if (data.timelineEvent != null) {
          const timelineEvent = data.timelineEvent
          setTimelineEvents((prev) => [
            timelineEvent,
            ...prev.filter((item) => item.id !== timelineEvent.id),
          ])
        }
        pushToast({
          level: 'success',
          title: `${previous.displayName}`,
          message: `${action} completed`,
        })
      } catch (error) {
        const fallback = previousServiceRef.current.get(serviceName)
        if (fallback) {
          setServices((prev) => upsertOpsService(prev, fallback))
        }
        pushToast({
          level: 'error',
          title: `${previous.displayName}`,
          message: error instanceof Error ? error.message : `${action} failed`,
        })
      } finally {
        previousServiceRef.current.delete(serviceName)
        setPendingActions((prev) => {
          const next = { ...prev }
          delete next[serviceName]
          return next
        })
      }
    },
    [api, pushToast, services],
  )

  const ackAlert = useCallback(
    async (alertID: number) => {
      const previous = alerts.find((item) => item.id === alertID)
      if (!previous) return

      setAlerts((prev) =>
        prev.map((item) =>
          item.id === alertID ? { ...item, status: 'acked' } : item,
        ),
      )

      try {
        const data = await api<{
          alert: OpsAlert
          timelineEvent?: OpsTimelineEvent
        }>(`/api/ops/alerts/${alertID}/ack`, {
          method: 'POST',
        })
        setAlerts((prev) =>
          prev.map((item) => (item.id === alertID ? data.alert : item)),
        )
        if (data.timelineEvent != null) {
          const timelineEvent = data.timelineEvent
          setTimelineEvents((prev) => [
            timelineEvent,
            ...prev.filter((item) => item.id !== timelineEvent.id),
          ])
        }
      } catch (error) {
        setAlerts((prev) =>
          prev.map((item) => (item.id === alertID ? previous : item)),
        )
        pushToast({
          level: 'error',
          title: previous.title,
          message: error instanceof Error ? error.message : 'failed to ack alert',
        })
      }
    },
    [alerts, api, pushToast],
  )

  const runRunbook = useCallback(
    async (runbookID: string) => {
      const runbook = runbooks.find((item) => item.id === runbookID)
      if (!runbook) return

      try {
        const data = await api<OpsRunbookRunResponse>(
          `/api/ops/runbooks/${encodeURIComponent(runbookID)}/run`,
          {
            method: 'POST',
          },
        )
        const job = data.job
        setJobs((prev) => {
          const next = prev.filter((item) => item.id !== job.id)
          return [job, ...next]
        })
        if (data.timelineEvent != null) {
          const timelineEvent = data.timelineEvent
          setTimelineEvents((prev) => [
            timelineEvent,
            ...prev.filter((item) => item.id !== timelineEvent.id),
          ])
        }
        pushToast({
          level: 'success',
          title: runbook.name,
          message: `run completed with status ${job.status}`,
        })
      } catch (error) {
        pushToast({
          level: 'error',
          title: runbook.name,
          message:
            error instanceof Error ? error.message : 'failed to run runbook',
        })
      }
    },
    [api, pushToast, runbooks],
  )

  const stats = useMemo(() => {
    if (overview == null) {
      return {
        host: '-',
        uptime: '-',
        services: '0/0',
        health: '-',
      }
    }
    const health =
      overview.services.failed > 0
        ? `${overview.services.failed} failed`
        : 'healthy'
    return {
      host: `${overview.host.hostname || '-'} (${overview.host.os}/${overview.host.arch})`,
      uptime: formatUptime(overview.sentinel.uptimeSec),
      services: `${overview.services.active}/${overview.services.total} active`,
      health,
    }
  }, [overview])

  return (
    <AppShell
      sidebar={
        <OpsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          loading={servicesLoading}
          error={servicesError}
          services={services}
          onTokenChange={setToken}
          onRefresh={refreshPage}
        />
      }
    >
      <main className="grid min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(34,197,94,.16),transparent_34%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <span className="truncate">Sentinel</span>
            <span className="text-muted-foreground">/</span>
            <span className="truncate text-muted-foreground">ops</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh ops"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <ScrollArea className="min-h-0">
          <section className="grid min-h-0 gap-3 p-3">
            <div className="grid grid-cols-1 gap-2 md:grid-cols-4">
              <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                  Host
                </p>
                <p className="mt-1 min-w-0 truncate text-[12px] font-semibold">
                  {stats.host}
                </p>
              </div>
              <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                  Uptime
                </p>
                <p className="mt-1 text-[12px] font-semibold">{stats.uptime}</p>
              </div>
              <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                  Services
                </p>
                <p className="mt-1 text-[12px] font-semibold">
                  {stats.services}
                </p>
              </div>
              <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                  Health
                </p>
                <p className="mt-1 text-[12px] font-semibold">{stats.health}</p>
              </div>
            </div>

            <section className="rounded-lg border border-border-subtle bg-secondary">
              <div className="flex items-center justify-between gap-2 border-b border-border-subtle px-3 py-2">
                <nav className="flex flex-wrap gap-1 rounded-md border border-border-subtle bg-surface-elevated p-1">
                  <button
                    type="button"
                    className={opsTabButtonClass(opsTab === 'services')}
                    onClick={() => setOpsTab('services')}
                  >
                    Services
                  </button>
                  <button
                    type="button"
                    className={opsTabButtonClass(opsTab === 'alerts')}
                    onClick={() => setOpsTab('alerts')}
                  >
                    <Bell className="h-3 w-3" />
                    Alerts
                    {alerts.length > 0 && (
                      <span
                        className={cn(
                          'ml-1 rounded-full px-1 text-[10px]',
                          opsTab === 'alerts'
                            ? 'bg-amber-400/20 text-amber-100'
                            : 'bg-amber-500/20 text-amber-200',
                        )}
                      >
                        {alerts.length}
                      </span>
                    )}
                  </button>
                  <button
                    type="button"
                    className={opsTabButtonClass(opsTab === 'timeline')}
                    onClick={() => setOpsTab('timeline')}
                  >
                    <Clock3 className="h-3 w-3" />
                    Timeline
                  </button>
                  <button
                    type="button"
                    className={opsTabButtonClass(opsTab === 'runbooks')}
                    onClick={() => setOpsTab('runbooks')}
                  >
                    Runbooks
                  </button>
                </nav>
                <span className="text-[10px] text-muted-foreground">
                  event-driven
                </span>
              </div>

              {opsTab === 'services' && (
                <div className="grid gap-1.5 p-2">
                  {services.map((service) => {
                    const pending = pendingActions[service.name]
                    const disabled = pending !== undefined
                    return (
                      <div
                        key={service.name}
                        className={cn(
                          'grid gap-2 rounded border px-2.5 py-2',
                          serviceRowTone(service),
                        )}
                      >
                        <div className="flex min-w-0 items-center justify-between gap-2">
                          <div className="min-w-0">
                            <p className="truncate text-[12px] font-semibold">
                              {service.displayName}
                            </p>
                            <p className="truncate text-[10px] text-muted-foreground">
                              {service.unit} • {service.scope}
                            </p>
                          </div>
                          <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                            {service.activeState}
                          </span>
                        </div>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() =>
                              runServiceAction(service.name, 'start')
                            }
                            disabled={disabled}
                          >
                            <Play className="h-3 w-3" />
                            Start
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => runServiceAction(service.name, 'stop')}
                            disabled={disabled}
                          >
                            <Square className="h-3 w-3" />
                            Stop
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() =>
                              runServiceAction(service.name, 'restart')
                            }
                            disabled={disabled}
                          >
                            <RotateCw className="h-3 w-3" />
                            Restart
                          </Button>
                          {pending && (
                            <span className="text-[10px] text-muted-foreground">
                              {pending}...
                            </span>
                          )}
                        </div>
                      </div>
                    )
                  })}
                  {!servicesLoading && services.length === 0 && (
                    <p className="p-2 text-[12px] text-muted-foreground">
                      No services available.
                    </p>
                  )}
                  {servicesError !== '' && (
                    <p className="px-2 pb-2 text-[12px] text-destructive-foreground">
                      {servicesError}
                    </p>
                  )}
                </div>
              )}

              {opsTab === 'alerts' && (
                <div className="grid gap-1.5 p-2">
                  {alerts.map((alert) => (
                    <div
                      key={alert.id}
                      className={cn(
                        'grid gap-2 rounded border px-2.5 py-2',
                        alert.severity === 'error'
                          ? 'border-red-500/45 bg-red-500/10'
                          : 'border-amber-500/45 bg-amber-500/10',
                      )}
                    >
                      <div className="flex min-w-0 items-center justify-between gap-2">
                        <div className="min-w-0">
                          <p className="truncate text-[12px] font-semibold">
                            {alert.title}
                          </p>
                          <p className="truncate text-[10px] text-muted-foreground">
                            {alert.resource} • {alert.occurrences}x
                          </p>
                        </div>
                        <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                          {alert.status}
                        </span>
                      </div>
                      <p className="text-[11px] text-muted-foreground">
                        {alert.message}
                      </p>
                      {alert.status === 'open' && (
                        <div>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => ackAlert(alert.id)}
                          >
                            Ack
                          </Button>
                        </div>
                      )}
                    </div>
                  ))}
                  {!alertsLoading && alerts.length === 0 && (
                    <p className="p-2 text-[12px] text-muted-foreground">
                      No active alerts.
                    </p>
                  )}
                  {alertsError !== '' && (
                    <p className="px-2 pb-2 text-[12px] text-destructive-foreground">
                      {alertsError}
                    </p>
                  )}
                </div>
              )}

              {opsTab === 'timeline' && (
                <div className="grid gap-2 p-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <input
                      value={timelineQuery}
                      onChange={(event) => setTimelineQuery(event.target.value)}
                      placeholder="search timeline"
                      className="h-8 min-w-52 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]"
                    />
                    <select
                      value={timelineSeverity}
                      onChange={(event) => setTimelineSeverity(event.target.value)}
                      className="h-8 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]"
                    >
                      <option value="all">all severities</option>
                      <option value="info">info</option>
                      <option value="warn">warn</option>
                      <option value="error">error</option>
                    </select>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 text-[11px]"
                      onClick={() => void refreshTimeline()}
                    >
                      <RefreshCw className="h-3 w-3" />
                      Filter
                    </Button>
                  </div>
                  <div className="grid gap-1.5">
                    {timelineEvents.map((event) => (
                      <div
                        key={event.id}
                        className="rounded border border-border-subtle bg-surface-elevated px-2.5 py-2"
                      >
                        <div className="flex min-w-0 items-center justify-between gap-2">
                          <p className="min-w-0 truncate text-[12px] font-semibold">
                            {event.message}
                          </p>
                          <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                            {event.severity}
                          </span>
                        </div>
                        <p className="mt-1 text-[10px] text-muted-foreground">
                          {event.source} • {event.resource} • {event.createdAt}
                        </p>
                        {event.details.trim() !== '' && (
                          <p className="mt-1 text-[11px] text-muted-foreground">
                            {event.details}
                          </p>
                        )}
                      </div>
                    ))}
                    {!timelineLoading && timelineEvents.length === 0 && (
                      <p className="p-2 text-[12px] text-muted-foreground">
                        No timeline events.
                      </p>
                    )}
                    {timelineError !== '' && (
                      <p className="px-2 pb-2 text-[12px] text-destructive-foreground">
                        {timelineError}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {opsTab === 'runbooks' && (
                <div className="grid gap-2 p-2">
                  {runbooks.map((runbook) => {
                    const lastJob = jobs.find(
                      (job) => job.runbookId === runbook.id,
                    )
                    return (
                      <div
                        key={runbook.id}
                        className="grid gap-2 rounded border border-border-subtle bg-surface-elevated px-2.5 py-2"
                      >
                        <div className="flex min-w-0 items-center justify-between gap-2">
                          <div className="min-w-0">
                            <p className="truncate text-[12px] font-semibold">
                              {runbook.name}
                            </p>
                            <p className="text-[11px] text-muted-foreground">
                              {runbook.description}
                            </p>
                          </div>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => runRunbook(runbook.id)}
                          >
                            Run
                          </Button>
                        </div>
                        <div className="text-[10px] text-muted-foreground">
                          {runbook.steps.length} steps
                        </div>
                        {lastJob && (
                          <div className="rounded border border-border-subtle bg-surface-overlay px-2 py-1 text-[10px] text-muted-foreground">
                            last run: {lastJob.status} • {lastJob.completedSteps}/
                            {lastJob.totalSteps} • {lastJob.createdAt}
                          </div>
                        )}
                      </div>
                    )
                  })}
                  {!runbooksLoading && runbooks.length === 0 && (
                    <p className="p-2 text-[12px] text-muted-foreground">
                      No runbooks available.
                    </p>
                  )}
                  {runbooksError !== '' && (
                    <p className="px-2 pb-2 text-[12px] text-destructive-foreground">
                      {runbooksError}
                    </p>
                  )}
                </div>
              )}
            </section>
          </section>
        </ScrollArea>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">
            {overviewError !== ''
              ? overviewError
              : overviewLoading
                ? 'Loading ops overview...'
                : 'Ops control plane connected'}
          </span>
          <span className="shrink-0 whitespace-nowrap">
            {overview?.updatedAt ? `updated ${overview.updatedAt}` : 'waiting'}
          </span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/ops')({
  component: OpsPage,
})
