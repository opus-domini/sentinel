import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Activity, Bell, Clock3, Menu, RefreshCw, Settings } from 'lucide-react'
import type {
  ConnectionState,
  OpsAlert,
  OpsAlertsResponse,
  OpsConfigResponse,
  OpsMetricsResponse,
  OpsOverview,
  OpsOverviewResponse,
  OpsTimelineEvent,
  OpsTimelineResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import AlertsSidebar from '@/components/AlertsSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { MetricCard } from '@/lib/MetricCard'
import {
  OPS_ALERTS_QUERY_KEY,
  OPS_CONFIG_QUERY_KEY,
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
} from '@/lib/opsQueryCache'
import {
  formatBytes,
  formatUptime,
  opsTabButtonClass,
  toErrorMessage,
} from '@/lib/opsUtils'
import { buildWSProtocols } from '@/lib/wsAuth'
import { cn } from '@/lib/utils'

function OpsPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [opsTab, setOpsTab] = useState<
    'alerts' | 'timeline' | 'metrics' | 'config'
  >('alerts')
  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('connecting')

  const [metricsAutoRefresh, setMetricsAutoRefresh] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [configEdited, setConfigEdited] = useState('')

  const timelineQueryKey = useMemo(
    () => opsTimelineQueryKey(timelineQuery, timelineSeverity),
    [timelineQuery, timelineSeverity],
  )
  const timelineQueryRef = useRef(timelineQuery)
  const timelineSeverityRef = useRef(timelineSeverity)
  useEffect(() => {
    timelineQueryRef.current = timelineQuery
  }, [timelineQuery])
  useEffect(() => {
    timelineSeverityRef.current = timelineSeverity
  }, [timelineSeverity])

  const overviewQuery = useQuery({
    queryKey: OPS_OVERVIEW_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsOverviewResponse>('/api/ops/overview')
      return data.overview
    },
  })

  const alertsQuery = useQuery({
    queryKey: OPS_ALERTS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsAlertsResponse>('/api/ops/alerts?limit=100')
      return data.alerts
    },
  })

  const timelineEventsQuery = useQuery({
    queryKey: timelineQueryKey,
    queryFn: async () => {
      const params = new URLSearchParams({ limit: '200' })
      if (timelineQuery.trim() !== '') params.set('q', timelineQuery.trim())
      if (timelineSeverity !== 'all') params.set('severity', timelineSeverity)
      const data = await api<OpsTimelineResponse>(
        `/api/ops/timeline?${params.toString()}`,
      )
      return data.events
    },
  })

  const metricsQuery = useQuery({
    queryKey: OPS_METRICS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsMetricsResponse>('/api/ops/metrics')
      return data.metrics
    },
    enabled: opsTab === 'metrics',
    refetchInterval: metricsAutoRefresh && opsTab === 'metrics' ? 5_000 : false,
  })

  const configQuery = useQuery({
    queryKey: OPS_CONFIG_QUERY_KEY,
    queryFn: async () => {
      return api<OpsConfigResponse>('/api/ops/config')
    },
    enabled: opsTab === 'config',
  })

  const overview = overviewQuery.data ?? null
  const alerts = alertsQuery.data ?? []
  const timelineEvents = timelineEventsQuery.data ?? []
  const metrics = metricsQuery.data ?? null
  const configContent = configQuery.data?.content ?? ''
  const configPath = configQuery.data?.path ?? ''

  const overviewLoading = overviewQuery.isLoading
  const alertsLoading = alertsQuery.isLoading
  const timelineLoading = timelineEventsQuery.isLoading
  const metricsLoading = metricsQuery.isLoading
  const configLoading = configQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const alertsError =
    alertsQuery.error != null
      ? toErrorMessage(alertsQuery.error, 'failed to load alerts')
      : ''
  const timelineError =
    timelineEventsQuery.error != null
      ? toErrorMessage(timelineEventsQuery.error, 'failed to load timeline')
      : ''
  const metricsError =
    metricsQuery.error != null
      ? toErrorMessage(metricsQuery.error, 'failed to load metrics')
      : ''
  const configError =
    configQuery.error != null
      ? toErrorMessage(configQuery.error, 'failed to load config')
      : ''

  const knownConfigContentRef = useRef('')
  useEffect(() => {
    if (configContent === '') {
      return
    }
    setConfigEdited((previous) =>
      previous === '' || previous === knownConfigContentRef.current
        ? configContent
        : previous,
    )
    knownConfigContentRef.current = configContent
  }, [configContent])

  const refreshOverview = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_OVERVIEW_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshAlerts = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_ALERTS_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshTimeline = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: opsTimelineQueryKey(
        timelineQueryRef.current,
        timelineSeverityRef.current,
      ),
      exact: true,
    })
  }, [queryClient])

  const refreshMetrics = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_METRICS_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshConfig = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_CONFIG_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const saveConfig = useCallback(async () => {
    setConfigSaving(true)
    try {
      await api('/api/ops/config', {
        method: 'PATCH',
        body: JSON.stringify({ content: configEdited }),
      })
      queryClient.setQueryData<OpsConfigResponse>(OPS_CONFIG_QUERY_KEY, {
        path: configPath,
        content: configEdited,
      })
      knownConfigContentRef.current = configEdited
      pushToast({
        title: 'Config',
        message: 'Saved (restart required)',
        level: 'info',
      })
    } catch (error) {
      pushToast({
        title: 'Config',
        message:
          error instanceof Error ? error.message : 'failed to save config',
        level: 'error',
      })
    } finally {
      setConfigSaving(false)
    }
  }, [api, configEdited, configPath, pushToast, queryClient])

  const refreshPage = useCallback(() => {
    void refreshOverview()
    void refreshAlerts()
    void refreshTimeline()
  }, [refreshAlerts, refreshOverview, refreshTimeline])

  useEffect(() => {
    if (tokenRequired && token.trim() === '') {
      setConnectionState('disconnected')
      return
    }

    let disposed = false
    let socket: WebSocket | null = null
    let retryTimer: number | null = null

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
            overview?: OpsOverview
            alerts?: Array<OpsAlert>
            event?: OpsTimelineEvent
          }
        }
        switch (typed.type) {
          case 'ops.overview.updated':
            if (
              typed.payload?.overview != null &&
              typeof typed.payload.overview === 'object'
            ) {
              queryClient.setQueryData(
                OPS_OVERVIEW_QUERY_KEY,
                typed.payload.overview,
              )
            } else {
              void refreshOverview()
            }
            break
          case 'ops.alerts.updated':
            if (Array.isArray(typed.payload?.alerts)) {
              queryClient.setQueryData(
                OPS_ALERTS_QUERY_KEY,
                typed.payload.alerts,
              )
            } else {
              void refreshAlerts()
            }
            break
          case 'ops.timeline.updated':
            if (typed.payload?.event != null) {
              const timelineEvent = typed.payload.event
              queryClient.setQueryData<Array<OpsTimelineEvent>>(
                opsTimelineQueryKey(
                  timelineQueryRef.current,
                  timelineSeverityRef.current,
                ),
                (previous = []) =>
                  prependOpsTimelineEvent(previous, timelineEvent),
              )
            } else {
              void refreshTimeline()
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
    queryClient,
    refreshAlerts,
    refreshOverview,
    refreshTimeline,
    token,
    tokenRequired,
  ])

  const ackAlert = useCallback(
    async (alertID: number) => {
      const previous = alerts.find((item) => item.id === alertID)
      if (!previous) return

      queryClient.setQueryData<Array<OpsAlert>>(
        OPS_ALERTS_QUERY_KEY,
        (current = []) =>
          current.map((item) =>
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
        queryClient.setQueryData<Array<OpsAlert>>(
          OPS_ALERTS_QUERY_KEY,
          (current = []) =>
            current.map((item) => (item.id === alertID ? data.alert : item)),
        )
        if (data.timelineEvent != null) {
          queryClient.setQueryData<Array<OpsTimelineEvent>>(
            opsTimelineQueryKey(
              timelineQueryRef.current,
              timelineSeverityRef.current,
            ),
            (current = []) =>
              prependOpsTimelineEvent(
                current,
                data.timelineEvent as OpsTimelineEvent,
              ),
          )
        }
      } catch (error) {
        queryClient.setQueryData<Array<OpsAlert>>(
          OPS_ALERTS_QUERY_KEY,
          (current = []) =>
            current.map((item) => (item.id === alertID ? previous : item)),
        )
        pushToast({
          level: 'error',
          title: previous.title,
          message:
            error instanceof Error ? error.message : 'failed to ack alert',
        })
      }
    },
    [alerts, api, pushToast, queryClient],
  )

  const stats = useMemo(() => {
    if (overview == null) {
      return {
        host: '-',
        uptime: '-',
        alerts: '0',
        health: '-',
      }
    }
    const health =
      overview.services.failed > 0
        ? `${overview.services.failed} failed`
        : 'healthy'
    const openAlerts = alerts.filter((a) => a.status === 'open').length
    return {
      host: `${overview.host.hostname || '-'} (${overview.host.os}/${overview.host.arch})`,
      uptime: formatUptime(overview.sentinel.uptimeSec),
      alerts: `${openAlerts} open`,
      health,
    }
  }, [overview, alerts])

  return (
    <AppShell
      sidebar={
        <AlertsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          loading={alertsLoading}
          alerts={alerts}
          onTokenChange={setToken}
          onAckAlert={(id) => void ackAlert(id)}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(34,197,94,.16),transparent_34%),var(--background)]">
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

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden p-3">
          <section>
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
              <MetricCard
                label="Alerts"
                value={stats.alerts}
                alert={alerts.some((a) => a.status === 'open')}
              />
              <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                  Health
                </p>
                <p className="mt-1 text-[12px] font-semibold">{stats.health}</p>
              </div>
            </div>
          </section>

          <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <div className="flex items-center justify-between gap-2 border-b border-border-subtle px-3 py-2">
              <nav className="flex flex-wrap gap-1 rounded-md border border-border-subtle bg-surface-elevated p-1">
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
                  className={opsTabButtonClass(opsTab === 'metrics')}
                  onClick={() => {
                    setOpsTab('metrics')
                    void refreshMetrics()
                  }}
                >
                  <Activity className="h-3 w-3" />
                  Metrics
                </button>
                <button
                  type="button"
                  className={opsTabButtonClass(opsTab === 'config')}
                  onClick={() => {
                    setOpsTab('config')
                    void refreshConfig()
                  }}
                >
                  <Settings className="h-3 w-3" />
                  Config
                </button>
              </nav>
              <span className="text-[10px] text-muted-foreground">
                event-driven
              </span>
            </div>

            {opsTab === 'alerts' && (
              <ScrollArea className="h-full min-h-0">
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
              </ScrollArea>
            )}

            {opsTab === 'timeline' && (
              <ScrollArea className="h-full min-h-0">
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
                      onChange={(event) =>
                        setTimelineSeverity(event.target.value)
                      }
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
              </ScrollArea>
            )}

            {opsTab === 'metrics' && (
              <ScrollArea className="h-full min-h-0">
                <div className="grid gap-2 p-2">
                  {metricsLoading && (
                    <p className="text-[12px] text-muted-foreground">
                      Loading metrics...
                    </p>
                  )}
                  {metricsError !== '' && (
                    <p className="text-[12px] text-destructive-foreground">
                      {metricsError}
                    </p>
                  )}
                  {!metricsLoading && metrics != null && (
                    <>
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => void refreshMetrics()}
                          >
                            <RefreshCw className="mr-1 h-3 w-3" />
                            Refresh
                          </Button>
                          <p className="text-[10px] text-muted-foreground">
                            {metrics.collectedAt}
                          </p>
                        </div>
                        <label className="flex cursor-pointer items-center gap-1.5 text-[11px] text-muted-foreground select-none">
                          <span>Auto</span>
                          <button
                            type="button"
                            role="switch"
                            aria-checked={metricsAutoRefresh}
                            onClick={() => setMetricsAutoRefresh((v) => !v)}
                            className={cn(
                              'relative inline-flex h-4 w-7 shrink-0 cursor-pointer items-center rounded-full border transition-colors',
                              metricsAutoRefresh
                                ? 'border-emerald-500/60 bg-emerald-500/30'
                                : 'border-border bg-surface-elevated',
                            )}
                          >
                            <span
                              className={cn(
                                'pointer-events-none block h-3 w-3 rounded-full bg-foreground shadow transition-transform',
                                metricsAutoRefresh
                                  ? 'translate-x-3'
                                  : 'translate-x-0',
                              )}
                            />
                          </button>
                          <span className="text-[10px]">5s</span>
                        </label>
                      </div>
                      <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
                        <MetricCard
                          label="CPU"
                          value={`${metrics.cpuPercent >= 0 ? metrics.cpuPercent.toFixed(1) : '—'}%`}
                          alert={metrics.cpuPercent > 90}
                        />
                        <MetricCard
                          label="Memory"
                          value={`${metrics.memPercent.toFixed(1)}%`}
                          sub={`${formatBytes(metrics.memUsedBytes)} / ${formatBytes(metrics.memTotalBytes)}`}
                          alert={metrics.memPercent > 90}
                        />
                        <MetricCard
                          label="Disk"
                          value={`${metrics.diskPercent.toFixed(1)}%`}
                          sub={`${formatBytes(metrics.diskUsedBytes)} / ${formatBytes(metrics.diskTotalBytes)}`}
                          alert={metrics.diskPercent > 95}
                        />
                        <MetricCard
                          label="Load Avg"
                          value={`${metrics.loadAvg1.toFixed(2)}`}
                          sub={`${metrics.loadAvg5.toFixed(2)} / ${metrics.loadAvg15.toFixed(2)}`}
                        />
                        <MetricCard
                          label="Goroutines"
                          value={`${metrics.numGoroutines}`}
                        />
                        <MetricCard
                          label="Go Heap"
                          value={`${metrics.goMemAllocMB.toFixed(1)} MB`}
                        />
                      </div>
                    </>
                  )}
                </div>
              </ScrollArea>
            )}

            {opsTab === 'config' && (
              <ScrollArea className="h-full min-h-0">
                <div className="grid gap-2 p-2">
                  {configLoading && (
                    <p className="text-[12px] text-muted-foreground">
                      Loading config...
                    </p>
                  )}
                  {configError !== '' && (
                    <p className="text-[12px] text-destructive-foreground">
                      {configError}
                    </p>
                  )}
                  {!configLoading && configContent !== '' && (
                    <>
                      <p className="text-[10px] text-muted-foreground">
                        {configPath}
                      </p>
                      <textarea
                        value={configEdited}
                        onChange={(e) => setConfigEdited(e.target.value)}
                        className="min-h-[300px] w-full rounded-md border border-border-subtle bg-background p-2 font-mono text-[11px] text-foreground focus:border-primary/60 focus:outline-none"
                        spellCheck={false}
                      />
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-[11px]"
                          disabled={
                            configSaving || configEdited === configContent
                          }
                          onClick={() => void saveConfig()}
                        >
                          {configSaving ? 'Saving...' : 'Save'}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 text-[11px]"
                          disabled={configEdited === configContent}
                          onClick={() => setConfigEdited(configContent)}
                        >
                          Reset
                        </Button>
                        <span className="text-[10px] text-muted-foreground">
                          {configEdited !== configContent
                            ? 'unsaved changes'
                            : 'no changes'}
                        </span>
                      </div>
                    </>
                  )}
                </div>
              </ScrollArea>
            )}
          </section>
        </div>

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
