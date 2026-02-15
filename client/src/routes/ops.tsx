import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  Activity,
  Bell,
  BookOpen,
  Clock3,
  FileText,
  Menu,
  Pin,
  PinOff,
  Play,
  RefreshCw,
  RotateCw,
  Search,
  Server,
  Settings,
  Square,
  X,
} from 'lucide-react'
import type {
  ConnectionState,
  OpsAlert,
  OpsAlertsResponse,
  OpsBrowseServicesResponse,
  OpsBrowsedService,
  OpsConfigResponse,
  OpsMetricsResponse,
  OpsOverview,
  OpsOverviewResponse,
  OpsRunbookRun,
  OpsRunbookRunResponse,
  OpsRunbooksResponse,
  OpsServiceAction,
  OpsServiceActionResponse,
  OpsServiceInspect,
  OpsServiceLogsResponse,
  OpsServiceStatus,
  OpsServiceStatusResponse,
  OpsServicesResponse,
  OpsTimelineEvent,
  OpsTimelineResponse,
  OpsUnitActionResponse,
  OpsUnitLogsResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import OpsSidebar from '@/components/OpsSidebar'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  canStartOpsService,
  canStopOpsService,
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'
import {
  OPS_ALERTS_QUERY_KEY,
  OPS_BROWSE_QUERY_KEY,
  OPS_CONFIG_QUERY_KEY,
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_RUNBOOKS_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
  upsertOpsRunbookJob,
} from '@/lib/opsQueryCache'
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

function browsedServiceDot(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') return 'bg-emerald-500'
  if (s === 'failed') return 'bg-red-500'
  return 'bg-muted-foreground/50'
}

function opsTabButtonClass(active: boolean): string {
  return cn(
    'inline-flex cursor-pointer items-center gap-1 rounded-md border px-2.5 py-1 text-[11px] font-medium transition-colors',
    active
      ? 'border-primary/40 bg-primary/15 text-primary-text-bright'
      : 'border-transparent text-muted-foreground hover:border-border hover:bg-surface-overlay hover:text-foreground',
  )
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  )
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return fallback
}

function MetricCard({
  label,
  value,
  sub,
  alert,
}: {
  label: string
  value: string
  sub?: string
  alert?: boolean
}) {
  return (
    <div
      className={cn(
        'rounded-lg border p-2.5',
        alert
          ? 'border-red-500/40 bg-red-500/10'
          : 'border-border-subtle bg-surface-elevated',
      )}
    >
      <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-[12px] font-semibold">{value}</p>
      {sub && <p className="text-[10px] text-muted-foreground">{sub}</p>}
    </div>
  )
}

function OpsPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [opsTab, setOpsTab] = useState<
    'services' | 'alerts' | 'timeline' | 'runbooks' | 'metrics' | 'config'
  >('services')
  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('connecting')
  const [, setPendingActions] = useState<
    Partial<Record<string, OpsServiceAction>>
  >({})
  const [serviceStatusOpen, setServiceStatusOpen] = useState(false)
  const [serviceStatusLoading, setServiceStatusLoading] = useState(false)
  const [serviceStatusError, setServiceStatusError] = useState('')
  const [serviceStatusData, setServiceStatusData] =
    useState<OpsServiceInspect | null>(null)

  const [metricsAutoRefresh, setMetricsAutoRefresh] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [configEdited, setConfigEdited] = useState('')
  const [serviceLogs, setServiceLogs] = useState('')
  const [serviceLogsLoading, setServiceLogsLoading] = useState(false)
  const [serviceLogsOpen, setServiceLogsOpen] = useState(false)
  const [serviceLogsTitle, setServiceLogsTitle] = useState('')

  const [svcStateFilter, setSvcStateFilter] = useState('all')
  const [svcScopeFilter, setSvcScopeFilter] = useState('all')
  const [svcSearch, setSvcSearch] = useState('')
  const [browsePendingActions, setBrowsePendingActions] = useState<
    Partial<Record<string, OpsServiceAction>>
  >({})

  const previousServiceRef = useRef(new Map<string, OpsServiceStatus>())

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

  const servicesQuery = useQuery({
    queryKey: OPS_SERVICES_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsServicesResponse>('/api/ops/services')
      return data.services
    },
  })

  const browseQuery = useQuery({
    queryKey: OPS_BROWSE_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsBrowseServicesResponse>(
        '/api/ops/services/browse',
      )
      return data.services
    },
    enabled: opsTab === 'services',
  })

  const alertsQuery = useQuery({
    queryKey: OPS_ALERTS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsAlertsResponse>('/api/ops/alerts?limit=100')
      return data.alerts
    },
  })

  const runbooksQuery = useQuery({
    queryKey: OPS_RUNBOOKS_QUERY_KEY,
    queryFn: async () => {
      return api<OpsRunbooksResponse>('/api/ops/runbooks')
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
  const services = servicesQuery.data ?? []
  const browseServices = browseQuery.data ?? []
  const alerts = alertsQuery.data ?? []
  const runbooks = runbooksQuery.data?.runbooks ?? []
  const jobs = runbooksQuery.data?.jobs ?? []
  const timelineEvents = timelineEventsQuery.data ?? []
  const metrics = metricsQuery.data ?? null
  const configContent = configQuery.data?.content ?? ''
  const configPath = configQuery.data?.path ?? ''

  const overviewLoading = overviewQuery.isLoading
  const servicesLoading = servicesQuery.isLoading
  const alertsLoading = alertsQuery.isLoading
  const timelineLoading = timelineEventsQuery.isLoading
  const runbooksLoading = runbooksQuery.isLoading
  const metricsLoading = metricsQuery.isLoading
  const configLoading = configQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const servicesError =
    servicesQuery.error != null
      ? toErrorMessage(servicesQuery.error, 'failed to load services')
      : ''
  const alertsError =
    alertsQuery.error != null
      ? toErrorMessage(alertsQuery.error, 'failed to load alerts')
      : ''
  const timelineError =
    timelineEventsQuery.error != null
      ? toErrorMessage(timelineEventsQuery.error, 'failed to load timeline')
      : ''
  const runbooksError =
    runbooksQuery.error != null
      ? toErrorMessage(runbooksQuery.error, 'failed to load runbooks')
      : ''
  const metricsError =
    metricsQuery.error != null
      ? toErrorMessage(metricsQuery.error, 'failed to load metrics')
      : ''
  const configError =
    configQuery.error != null
      ? toErrorMessage(configQuery.error, 'failed to load config')
      : ''

  const browseLoading = browseQuery.isLoading
  const browseError =
    browseQuery.error != null
      ? toErrorMessage(browseQuery.error, 'failed to browse services')
      : ''

  const filteredBrowseServices = useMemo(() => {
    let list = browseServices
    if (svcStateFilter !== 'all') {
      list = list.filter((s) => {
        const state = s.activeState.trim().toLowerCase()
        if (svcStateFilter === 'active')
          return state === 'active' || state === 'running'
        if (svcStateFilter === 'failed') return state === 'failed'
        if (svcStateFilter === 'inactive')
          return state === 'inactive' || state === 'dead'
        return true
      })
    }
    if (svcScopeFilter !== 'all') {
      list = list.filter(
        (s) => s.scope.toLowerCase() === svcScopeFilter.toLowerCase(),
      )
    }
    if (svcSearch.trim() !== '') {
      const q = svcSearch.trim().toLowerCase()
      list = list.filter(
        (s) =>
          s.unit.toLowerCase().includes(q) ||
          s.description.toLowerCase().includes(q),
      )
    }
    return list
  }, [browseServices, svcStateFilter, svcScopeFilter, svcSearch])

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

  const refreshServices = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_SERVICES_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshBrowse = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_BROWSE_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshAlerts = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_ALERTS_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshRunbooks = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_RUNBOOKS_QUERY_KEY,
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

  const refreshAll = useCallback(() => {
    void refreshOverview()
    void refreshServices()
    void refreshBrowse()
    void refreshAlerts()
    void refreshRunbooks()
  }, [
    refreshAlerts,
    refreshBrowse,
    refreshOverview,
    refreshRunbooks,
    refreshServices,
  ])

  const refreshPage = useCallback(() => {
    refreshAll()
    void refreshTimeline()
  }, [refreshAll, refreshTimeline])

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
              queryClient.setQueryData(
                OPS_SERVICES_QUERY_KEY,
                typed.payload.services,
              )
            } else {
              void refreshServices()
            }
            void refreshBrowse()
            break
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
          case 'ops.job.updated':
            if (typed.payload?.job != null) {
              const job = typed.payload.job
              queryClient.setQueryData<OpsRunbooksResponse>(
                OPS_RUNBOOKS_QUERY_KEY,
                (previous) => {
                  if (previous == null) return previous
                  return {
                    ...previous,
                    jobs: upsertOpsRunbookJob(previous.jobs, job),
                  }
                },
              )
            } else {
              void refreshRunbooks()
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
    refreshBrowse,
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
      queryClient.setQueryData<Array<OpsServiceStatus>>(
        OPS_SERVICES_QUERY_KEY,
        (current = []) =>
          current.map((item) =>
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
          queryClient.setQueryData(OPS_SERVICES_QUERY_KEY, data.services)
        } else {
          queryClient.setQueryData<Array<OpsServiceStatus>>(
            OPS_SERVICES_QUERY_KEY,
            (current = []) => upsertOpsService(current, data.service),
          )
        }
        queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, data.overview)
        if (Array.isArray(data.alerts)) {
          queryClient.setQueryData(OPS_ALERTS_QUERY_KEY, data.alerts)
        }
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
        pushToast({
          level: 'success',
          title: `${previous.displayName}`,
          message: `${action} completed`,
        })
      } catch (error) {
        const fallback = previousServiceRef.current.get(serviceName)
        if (fallback) {
          queryClient.setQueryData<Array<OpsServiceStatus>>(
            OPS_SERVICES_QUERY_KEY,
            (current = []) => upsertOpsService(current, fallback),
          )
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
    [api, pushToast, queryClient, services],
  )

  const unregisterService = useCallback(
    async (name: string) => {
      const previous = services.find((s) => s.name === name)
      queryClient.setQueryData<Array<OpsServiceStatus>>(
        OPS_SERVICES_QUERY_KEY,
        (current = []) => current.filter((service) => service.name !== name),
      )
      try {
        await api<{ removed: string; globalRev: number }>(
          `/api/ops/services/${encodeURIComponent(name)}`,
          { method: 'DELETE' },
        )
        pushToast({
          level: 'success',
          title: previous?.displayName ?? name,
          message: 'Service removed',
        })
        void refreshBrowse()
      } catch (error) {
        if (previous) {
          queryClient.setQueryData<Array<OpsServiceStatus>>(
            OPS_SERVICES_QUERY_KEY,
            (current = []) => [...current, previous],
          )
        }
        pushToast({
          level: 'error',
          title: 'Remove service',
          message: error instanceof Error ? error.message : 'failed to remove',
        })
      }
    },
    [api, pushToast, queryClient, refreshBrowse, services],
  )

  const actOnBrowsedService = useCallback(
    async (svc: OpsBrowsedService, action: OpsServiceAction) => {
      setBrowsePendingActions((prev) => ({ ...prev, [svc.unit]: action }))
      try {
        if (svc.tracked && svc.trackedName) {
          await runServiceAction(svc.trackedName, action)
        } else {
          const data = await api<OpsUnitActionResponse>(
            '/api/ops/services/unit/action',
            {
              method: 'POST',
              body: JSON.stringify({
                unit: svc.unit,
                scope: svc.scope,
                manager: svc.manager,
                action,
              }),
            },
          )
          queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, data.overview)
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
          pushToast({
            level: 'success',
            title: svc.unit,
            message: `${action} completed`,
          })
        }
        void refreshBrowse()
      } catch (error) {
        pushToast({
          level: 'error',
          title: svc.unit,
          message: error instanceof Error ? error.message : `${action} failed`,
        })
      } finally {
        setBrowsePendingActions((prev) => {
          const next = { ...prev }
          delete next[svc.unit]
          return next
        })
      }
    },
    [api, pushToast, queryClient, refreshBrowse, runServiceAction],
  )

  const inspectBrowsedService = useCallback(
    async (svc: OpsBrowsedService) => {
      setServiceStatusOpen(true)
      setServiceStatusLoading(true)
      setServiceStatusError('')
      try {
        if (svc.tracked && svc.trackedName) {
          const data = await api<OpsServiceStatusResponse>(
            `/api/ops/services/${encodeURIComponent(svc.trackedName)}/status`,
          )
          setServiceStatusData(data.status)
        } else {
          const params = new URLSearchParams({
            unit: svc.unit,
            scope: svc.scope,
            manager: svc.manager,
          })
          const data = await api<OpsServiceStatusResponse>(
            `/api/ops/services/unit/status?${params.toString()}`,
          )
          setServiceStatusData(data.status)
        }
      } catch (error) {
        setServiceStatusData(null)
        setServiceStatusError(
          error instanceof Error
            ? error.message
            : 'failed to load service status',
        )
      } finally {
        setServiceStatusLoading(false)
      }
    },
    [api],
  )

  const fetchBrowsedServiceLogs = useCallback(
    async (svc: OpsBrowsedService) => {
      setServiceLogsOpen(true)
      setServiceLogsTitle(svc.unit)
      setServiceLogsLoading(true)
      setServiceLogs('')
      try {
        if (svc.tracked && svc.trackedName) {
          const data = await api<OpsServiceLogsResponse>(
            `/api/ops/services/${encodeURIComponent(svc.trackedName)}/logs?lines=200`,
          )
          setServiceLogs(data.output)
        } else {
          const params = new URLSearchParams({
            unit: svc.unit,
            scope: svc.scope,
            manager: svc.manager,
            lines: '200',
          })
          const data = await api<OpsUnitLogsResponse>(
            `/api/ops/services/unit/logs?${params.toString()}`,
          )
          setServiceLogs(data.output)
        }
      } catch {
        setServiceLogs('(failed to fetch logs)')
      } finally {
        setServiceLogsLoading(false)
      }
    },
    [api],
  )

  const toggleTrack = useCallback(
    async (svc: OpsBrowsedService) => {
      if (svc.tracked && svc.trackedName) {
        await unregisterService(svc.trackedName)
      } else {
        const name = svc.unit
          .replace(/\.(service|timer|socket|mount|slice)$/, '')
          .replace(/\./g, '-')
        try {
          const data = await api<{
            services: Array<OpsServiceStatus>
            globalRev: number
          }>('/api/ops/services', {
            method: 'POST',
            body: JSON.stringify({
              name,
              displayName: svc.description || svc.unit,
              manager: svc.manager,
              unit: svc.unit,
              scope: svc.scope,
            }),
          })
          if (Array.isArray(data.services)) {
            queryClient.setQueryData(OPS_SERVICES_QUERY_KEY, data.services)
          }
          pushToast({
            level: 'success',
            title: svc.description || svc.unit,
            message: 'Service tracked',
          })
          void refreshBrowse()
        } catch (error) {
          pushToast({
            level: 'error',
            title: 'Track service',
            message: error instanceof Error ? error.message : 'failed to track',
          })
        }
      }
    },
    [api, pushToast, queryClient, refreshBrowse, unregisterService],
  )

  const navigateToService = useCallback((unit: string) => {
    setOpsTab('services')
    setSvcStateFilter('all')
    setSvcScopeFilter('all')
    setSvcSearch(unit)
  }, [])

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
        queryClient.setQueryData<OpsRunbooksResponse>(
          OPS_RUNBOOKS_QUERY_KEY,
          (previous) => {
            if (previous == null) return previous
            return {
              ...previous,
              jobs: upsertOpsRunbookJob(previous.jobs, job),
            }
          },
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
    [api, pushToast, queryClient, runbooks],
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
          onRemoveService={unregisterService}
          onNavigateToService={navigateToService}
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
          </section>

          <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <div className="flex items-center justify-between gap-2 border-b border-border-subtle px-3 py-2">
              <nav className="flex flex-wrap gap-1 rounded-md border border-border-subtle bg-surface-elevated p-1">
                <button
                  type="button"
                  className={opsTabButtonClass(opsTab === 'services')}
                  onClick={() => setOpsTab('services')}
                >
                  <Server className="h-3 w-3" />
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
                  <BookOpen className="h-3 w-3" />
                  Runbooks
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

            {opsTab === 'services' && (
              <div className="grid min-h-0 grid-rows-[auto_1fr] gap-2 overflow-hidden p-2">
                <div className="flex flex-wrap items-center gap-2">
                  <select
                    value={svcStateFilter}
                    onChange={(e) => setSvcStateFilter(e.target.value)}
                    className="h-8 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]"
                  >
                    <option value="all">All states</option>
                    <option value="active">Active</option>
                    <option value="inactive">Inactive</option>
                    <option value="failed">Failed</option>
                  </select>
                  <select
                    value={svcScopeFilter}
                    onChange={(e) => setSvcScopeFilter(e.target.value)}
                    className="h-8 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]"
                  >
                    <option value="all">All scopes</option>
                    <option value="user">user</option>
                    <option value="system">system</option>
                  </select>
                  <div className="relative min-w-44 flex-1">
                    <Search className="absolute left-2 top-2 h-4 w-4 text-muted-foreground" />
                    <input
                      value={svcSearch}
                      onChange={(e) => setSvcSearch(e.target.value)}
                      placeholder="Search services..."
                      className={cn(
                        'h-8 w-full rounded-md border border-border-subtle bg-surface-overlay pl-8 text-[12px] placeholder:text-muted-foreground',
                        svcSearch ? 'pr-7' : 'pr-2',
                      )}
                    />
                    {svcSearch && (
                      <button
                        type="button"
                        className="absolute right-1.5 top-1.5 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground"
                        onClick={() => setSvcSearch('')}
                        aria-label="Clear search"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                  <TooltipHelper content="Refresh service list">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 cursor-pointer text-[11px]"
                      onClick={() => void refreshBrowse()}
                    >
                      <RefreshCw className="h-3 w-3" />
                    </Button>
                  </TooltipHelper>
                  <span className="text-[10px] text-muted-foreground">
                    {filteredBrowseServices.length}/{browseServices.length}{' '}
                    services
                  </span>
                </div>
                <ScrollArea className="h-full min-h-0">
                  <div className="grid gap-1">
                    {filteredBrowseServices.map((svc) => {
                      const pending = browsePendingActions[svc.unit]
                      const rowBusy = pending !== undefined
                      const startDisabled = rowBusy || !canStartOpsService(svc)
                      const stopDisabled = rowBusy || !canStopOpsService(svc)
                      return (
                        <div
                          key={`${svc.scope}:${svc.unit}`}
                          className="grid min-w-0 gap-2 rounded border border-border-subtle bg-surface-elevated px-2.5 py-2"
                        >
                          <div className="flex min-w-0 items-start gap-2">
                            <span
                              className={cn(
                                'mt-1 h-2 w-2 shrink-0 rounded-full',
                                browsedServiceDot(svc.activeState),
                              )}
                            />
                            <div className="min-w-0 flex-1">
                              <div className="flex min-w-0 items-center gap-1.5">
                                <p className="min-w-0 flex-1 truncate text-[12px] font-medium">
                                  {svc.unit}
                                </p>
                                <div className="flex shrink-0 items-center gap-1.5">
                                  <span className="rounded border border-border-subtle px-1 text-[9px] text-muted-foreground">
                                    {svc.scope}
                                  </span>
                                  <span className="text-[10px] text-muted-foreground">
                                    {svc.activeState}
                                  </span>
                                </div>
                              </div>
                              {svc.description &&
                                svc.description !== svc.unit && (
                                  <p className="truncate text-[10px] text-muted-foreground">
                                    {svc.description}
                                  </p>
                                )}
                            </div>
                            {pending && (
                              <span className="shrink-0 text-[10px] text-muted-foreground">
                                {pending}...
                              </span>
                            )}
                          </div>
                          <div className="flex flex-wrap items-center justify-center gap-1.5 pl-4">
                            <TooltipHelper content="Start service">
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 cursor-pointer gap-1 px-2 text-[11px]"
                                onClick={() =>
                                  actOnBrowsedService(svc, 'start')
                                }
                                disabled={startDisabled}
                                aria-label="Start service"
                              >
                                <Play className="h-3 w-3" />
                                Start
                              </Button>
                            </TooltipHelper>
                            <TooltipHelper content="Stop service">
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 cursor-pointer gap-1 px-2 text-[11px]"
                                onClick={() => actOnBrowsedService(svc, 'stop')}
                                disabled={stopDisabled}
                                aria-label="Stop service"
                              >
                                <Square className="h-3 w-3" />
                                Stop
                              </Button>
                            </TooltipHelper>
                            <TooltipHelper content="Restart service">
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 cursor-pointer gap-1 px-2 text-[11px]"
                                onClick={() =>
                                  actOnBrowsedService(svc, 'restart')
                                }
                                disabled={rowBusy}
                                aria-label="Restart service"
                              >
                                <RotateCw className="h-3 w-3" />
                                Restart
                              </Button>
                            </TooltipHelper>
                            <TooltipHelper content="Inspect status">
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 cursor-pointer gap-1 px-2 text-[11px]"
                                onClick={() => void inspectBrowsedService(svc)}
                                disabled={rowBusy}
                                aria-label="Inspect service status"
                              >
                                <FileText className="h-3 w-3" />
                                Status
                              </Button>
                            </TooltipHelper>
                            <TooltipHelper content="View logs">
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 cursor-pointer gap-1 px-2 text-[11px]"
                                onClick={() =>
                                  void fetchBrowsedServiceLogs(svc)
                                }
                                disabled={rowBusy}
                                aria-label="View service logs"
                              >
                                <Clock3 className="h-3 w-3" />
                                Logs
                              </Button>
                            </TooltipHelper>
                            <TooltipHelper
                              content={
                                svc.tracked
                                  ? 'Unpin from sidebar'
                                  : 'Pin to sidebar'
                              }
                            >
                              <Button
                                variant="outline"
                                size="sm"
                                className={cn(
                                  'h-7 cursor-pointer gap-1 px-2 text-[11px]',
                                  svc.tracked ? 'text-primary-text-bright' : '',
                                )}
                                onClick={() => void toggleTrack(svc)}
                                disabled={rowBusy}
                                aria-label={svc.tracked ? 'Unpin service' : 'Pin service'}
                              >
                                {svc.tracked ? (
                                  <PinOff className="h-3 w-3" />
                                ) : (
                                  <Pin className="h-3 w-3" />
                                )}
                                {svc.tracked ? 'Unpin' : 'Pin'}
                              </Button>
                            </TooltipHelper>
                          </div>
                        </div>
                      )
                    })}
                    {!browseLoading && filteredBrowseServices.length === 0 && (
                      <p className="p-2 text-[12px] text-muted-foreground">
                        {browseServices.length === 0
                          ? 'No services discovered on this host.'
                          : 'No services match filters.'}
                      </p>
                    )}
                    {browseError !== '' && (
                      <p className="px-2 pb-2 text-[12px] text-destructive-foreground">
                        {browseError}
                      </p>
                    )}
                  </div>
                </ScrollArea>
              </div>
            )}

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

            {opsTab === 'runbooks' && (
              <ScrollArea className="h-full min-h-0">
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
                            last run: {lastJob.status} •{' '}
                            {lastJob.completedSteps}/{lastJob.totalSteps} •{' '}
                            {lastJob.createdAt}
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

      <Dialog open={serviceStatusOpen} onOpenChange={setServiceStatusOpen}>
        <DialogContent className="max-h-[85vh] max-w-[calc(100vw-1rem)] overflow-hidden sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {serviceStatusData?.service.unit ?? 'Service status'}
            </DialogTitle>
            <DialogDescription>
              {serviceStatusData?.summary ??
                'Runtime details from service manager'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 gap-2 overflow-hidden">
            {serviceStatusLoading && (
              <p className="text-[12px] text-muted-foreground">
                Loading service status...
              </p>
            )}
            {serviceStatusError !== '' && (
              <p className="rounded-md border border-destructive/40 bg-destructive/10 px-2 py-1 text-[12px] text-destructive-foreground">
                {serviceStatusError}
              </p>
            )}

            {!serviceStatusLoading && serviceStatusData != null && (
              <ScrollArea className="max-h-[58vh] min-h-0">
                <div className="grid gap-2 pr-2">
                  <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                    <p className="text-[11px] font-semibold text-foreground">
                      {serviceStatusData.service.unit}
                    </p>
                    <p className="text-[10px] text-muted-foreground">
                      checked at {serviceStatusData.checkedAt}
                    </p>
                  </div>

                  {serviceStatusData.properties != null &&
                    Object.keys(serviceStatusData.properties).length > 0 && (
                      <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                        <p className="mb-1 text-[11px] font-semibold text-foreground">
                          Properties
                        </p>
                        <div className="grid gap-1 text-[11px]">
                          {Object.entries(serviceStatusData.properties)
                            .sort(([a], [b]) => a.localeCompare(b))
                            .map(([key, value]) => (
                              <div
                                key={key}
                                className="grid grid-cols-[9rem_1fr] gap-2"
                              >
                                <span className="font-mono text-muted-foreground">
                                  {key}
                                </span>
                                <span className="break-all font-mono text-foreground">
                                  {value}
                                </span>
                              </div>
                            ))}
                        </div>
                      </div>
                    )}

                  {serviceStatusData.output?.trim() !== '' && (
                    <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                      <p className="mb-1 text-[11px] font-semibold text-foreground">
                        Raw output
                      </p>
                      <pre className="max-h-[36vh] overflow-auto whitespace-pre-wrap break-words rounded border border-border-subtle bg-background p-2 font-mono text-[11px] text-secondary-foreground">
                        {serviceStatusData.output}
                      </pre>
                    </div>
                  )}
                </div>
              </ScrollArea>
            )}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={serviceLogsOpen} onOpenChange={setServiceLogsOpen}>
        <DialogContent className="max-h-[85vh] max-w-[calc(100vw-1rem)] overflow-hidden sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{serviceLogsTitle || 'Service logs'}</DialogTitle>
            <DialogDescription>Recent log output</DialogDescription>
          </DialogHeader>
          <div className="grid min-h-0 gap-2 overflow-hidden">
            {serviceLogsLoading && (
              <p className="text-[12px] text-muted-foreground">
                Loading logs...
              </p>
            )}
            {!serviceLogsLoading && serviceLogs !== '' && (
              <div className="max-h-[60vh] min-h-0 overflow-auto rounded border border-border-subtle bg-background">
                <pre className="whitespace-pre p-2 font-mono text-[11px] text-secondary-foreground">
                  {serviceLogs}
                </pre>
              </div>
            )}
            {!serviceLogsLoading && serviceLogs === '' && (
              <p className="text-[12px] text-muted-foreground">
                No logs available.
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </AppShell>
  )
}

export const Route = createFileRoute('/ops')({
  component: OpsPage,
})
