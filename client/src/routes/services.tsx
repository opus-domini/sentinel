import { useCallback, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  AlertTriangle,
  ArrowDownToLine,
  CheckCircle2,
  Clock3,
  FileText,
  Layers,
  Menu,
  Pin,
  PinOff,
  Play,
  Radio,
  RefreshCw,
  RotateCw,
  Search,
  Square,
  WrapText,
  X,
} from 'lucide-react'
import type {
  OpsBrowseServicesResponse,
  OpsBrowsedService,
  OpsOverview,
  OpsOverviewResponse,
  OpsServiceAction,
  OpsServiceActionResponse,
  OpsServiceInspect,
  OpsServiceLogsResponse,
  OpsServiceStatus,
  OpsServiceStatusResponse,
  OpsServicesResponse,
  OpsTimelineEvent,
  OpsUnitActionResponse,
  OpsUnitLogsResponse,
} from '@/types'
import type { ParsedLogLine } from '@/lib/log-parser'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { LogViewer } from '@/components/LogViewer'
import ServicesSidebar from '@/components/ServicesSidebar'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useLogStream } from '@/hooks/useLogStream'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { parseLogLines, parseSingleLine } from '@/lib/log-parser'
import {
  canStartOpsService,
  canStopOpsService,
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'
import { MetricCard } from '@/lib/MetricCard'
import {
  OPS_ALERTS_QUERY_KEY,
  OPS_BROWSE_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
} from '@/lib/opsQueryCache'
import { browsedServiceDot, toErrorMessage } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

const LOG_BUFFER_MAX = 5_000

function ServicesPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [, setPendingActions] = useState<
    Partial<Record<string, OpsServiceAction>>
  >({})
  const [serviceStatusOpen, setServiceStatusOpen] = useState(false)
  const [serviceStatusLoading, setServiceStatusLoading] = useState(false)
  const [serviceStatusError, setServiceStatusError] = useState('')
  const [serviceStatusData, setServiceStatusData] =
    useState<OpsServiceInspect | null>(null)

  const [serviceLogLines, setServiceLogLines] = useState<
    Array<ParsedLogLine>
  >([])
  const [serviceLogsLoading, setServiceLogsLoading] = useState(false)
  const [serviceLogsOpen, setServiceLogsOpen] = useState(false)
  const [serviceLogsTitle, setServiceLogsTitle] = useState('')
  const [serviceLogsSearch, setServiceLogsSearch] = useState('')
  const [serviceLogsWrap, setServiceLogsWrap] = useState(false)
  const [serviceLogsFollow, setServiceLogsFollow] = useState(true)
  const [streamEnabled, setStreamEnabled] = useState(false)
  const serviceLogsServiceRef = useRef<OpsBrowsedService | null>(null)
  const lineCounterRef = useRef(0)

  const [svcStateFilter, setSvcStateFilter] = useState('all')
  const [svcScopeFilter, setSvcScopeFilter] = useState('all')
  const [svcSearch, setSvcSearch] = useState('')
  const [browsePendingActions, setBrowsePendingActions] = useState<
    Partial<Record<string, OpsServiceAction>>
  >({})

  const previousServiceRef = useRef(new Map<string, OpsServiceStatus>())

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
  })

  const overview = overviewQuery.data ?? null
  const services = servicesQuery.data ?? []
  const browseServices = browseQuery.data ?? []

  const servicesLoading = servicesQuery.isLoading
  const servicesError =
    servicesQuery.error != null
      ? toErrorMessage(servicesQuery.error, 'failed to load services')
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

  const refreshOverview = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_OVERVIEW_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshPage = useCallback(() => {
    void refreshServices()
    void refreshBrowse()
    void refreshOverview()
  }, [refreshServices, refreshBrowse, refreshOverview])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      const typed = message as {
        type?: string
        payload?: {
          services?: Array<OpsServiceStatus>
          overview?: OpsOverview
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
        default:
          break
      }
    },
    [queryClient, refreshBrowse, refreshOverview, refreshServices],
  )

  const connectionState = useOpsEventsSocket({
    token,
    tokenRequired,
    onMessage: handleWSMessage,
  })

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
            opsTimelineQueryKey('', 'all'),
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
              opsTimelineQueryKey('', 'all'),
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
      serviceLogsServiceRef.current = svc
      setServiceLogsOpen(true)
      setServiceLogsTitle(svc.unit)
      setServiceLogsLoading(true)
      setServiceLogLines([])
      setServiceLogsSearch('')
      setServiceLogsFollow(true)
      setStreamEnabled(false)
      lineCounterRef.current = 0
      try {
        let output = ''
        if (svc.tracked && svc.trackedName) {
          const data = await api<OpsServiceLogsResponse>(
            `/api/ops/services/${encodeURIComponent(svc.trackedName)}/logs?lines=200`,
          )
          output = data.output
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
          output = data.output
        }
        const parsed = parseLogLines(output)
        lineCounterRef.current = parsed.length
        setServiceLogLines(parsed)
        setStreamEnabled(true)
      } catch {
        setServiceLogLines([
          {
            lineNumber: 1,
            raw: '(failed to fetch logs)',
            timestamp: '',
            hostname: '',
            unit: '',
            message: '(failed to fetch logs)',
            level: 'error',
          },
        ])
      } finally {
        setServiceLogsLoading(false)
      }
    },
    [api],
  )

  const refreshServiceLogs = useCallback(async () => {
    const svc = serviceLogsServiceRef.current
    if (!svc) return
    setStreamEnabled(false)
    try {
      let output = ''
      if (svc.tracked && svc.trackedName) {
        const data = await api<OpsServiceLogsResponse>(
          `/api/ops/services/${encodeURIComponent(svc.trackedName)}/logs?lines=200`,
        )
        output = data.output
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
        output = data.output
      }
      const parsed = parseLogLines(output)
      lineCounterRef.current = parsed.length
      setServiceLogLines(parsed)
      setServiceLogsFollow(true)
      setStreamEnabled(true)
    } catch {
      // keep existing lines on refresh failure
    }
  }, [api])

  const handleStreamLine = useCallback((line: string) => {
    lineCounterRef.current += 1
    const parsed = parseSingleLine(line, lineCounterRef.current)
    setServiceLogLines((prev) => {
      const next = [...prev, parsed]
      if (next.length > LOG_BUFFER_MAX) {
        return next.slice(next.length - LOG_BUFFER_MAX)
      }
      return next
    })
  }, [])

  const streamTarget = useMemo(() => {
    const svc = serviceLogsServiceRef.current
    if (!svc || !serviceLogsOpen) return null
    if (svc.tracked && svc.trackedName) {
      return { kind: 'service' as const, name: svc.trackedName }
    }
    return {
      kind: 'unit' as const,
      unit: svc.unit,
      scope: svc.scope,
      manager: svc.manager,
    }
  }, [serviceLogsOpen])

  const streamStatus = useLogStream({
    token,
    tokenRequired,
    target: streamTarget,
    enabled: streamEnabled && serviceLogsOpen,
    onLine: handleStreamLine,
  })

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
    setSvcStateFilter('all')
    setSvcScopeFilter('all')
    setSvcSearch(unit)
  }, [])

  const stats = useMemo(() => {
    if (overview == null) {
      return { total: '0', active: '0', failed: '0' }
    }
    return {
      total: `${overview.services.total}`,
      active: `${overview.services.active}`,
      failed: `${overview.services.failed}`,
    }
  }, [overview])

  return (
    <AppShell
      sidebar={
        <ServicesSidebar
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
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(6,182,212,.16),transparent_34%),var(--background)]">
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
            <span className="truncate text-muted-foreground">services</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh services"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-2 overflow-hidden p-2 md:gap-3 md:p-3">
          <section>
            <div className="hidden gap-2 md:grid md:grid-cols-3">
              <MetricCard label="Total" value={stats.total} />
              <MetricCard label="Active" value={stats.active} />
              <MetricCard
                label="Failed"
                value={stats.failed}
                alert={Number(stats.failed) > 0}
              />
            </div>
            <div className="flex items-center justify-center gap-4 rounded-lg border border-border-subtle bg-surface-elevated px-2 py-1.5 md:hidden">
              <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Layers className="h-3.5 w-3.5" />
                <span className="font-semibold text-foreground">
                  {stats.total}
                </span>
                total
              </span>
              <span className="flex items-center gap-1.5 text-[11px] text-emerald-400">
                <CheckCircle2 className="h-3.5 w-3.5" />
                <span className="font-semibold">{stats.active}</span>
                active
              </span>
              <span
                className={cn(
                  'flex items-center gap-1.5 text-[11px]',
                  Number(stats.failed) > 0
                    ? 'text-red-400'
                    : 'text-muted-foreground',
                )}
              >
                <AlertTriangle className="h-3.5 w-3.5" />
                <span className="font-semibold">{stats.failed}</span>
                failed
              </span>
            </div>
          </section>

          <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <div className="flex flex-wrap items-center gap-2 border-b border-border-subtle p-2">
              <select
                value={svcStateFilter}
                onChange={(e) => setSvcStateFilter(e.target.value)}
                className="h-7 flex-1 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px] md:h-8 md:flex-none"
              >
                <option value="all">All states</option>
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
                <option value="failed">Failed</option>
              </select>
              <select
                value={svcScopeFilter}
                onChange={(e) => setSvcScopeFilter(e.target.value)}
                className="h-7 flex-1 rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px] md:h-8 md:flex-none"
              >
                <option value="all">All scopes</option>
                <option value="user">user</option>
                <option value="system">system</option>
              </select>
              <div className="flex w-full items-center gap-2 md:w-auto md:min-w-44 md:flex-1">
                <div className="relative min-w-0 flex-1">
                  <Search className="absolute left-2 top-1.5 h-4 w-4 text-muted-foreground md:top-2" />
                  <input
                    value={svcSearch}
                    onChange={(e) => setSvcSearch(e.target.value)}
                    placeholder="Search services..."
                    className={cn(
                      'h-7 w-full rounded-md border border-border-subtle bg-surface-overlay pl-8 text-[12px] placeholder:text-muted-foreground md:h-8',
                      svcSearch ? 'pr-7' : 'pr-2',
                    )}
                  />
                  {svcSearch && (
                    <button
                      type="button"
                      className="absolute right-1.5 top-1 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground md:top-1.5"
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
                    className="h-7 cursor-pointer text-[11px] md:h-8"
                    onClick={() => void refreshBrowse()}
                  >
                    <RefreshCw className="h-3 w-3" />
                  </Button>
                </TooltipHelper>
              </div>
              <span className="hidden text-[10px] text-muted-foreground md:inline">
                {filteredBrowseServices.length}/{browseServices.length} services
              </span>
            </div>
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-1 p-2">
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
                          {svc.description && svc.description !== svc.unit && (
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
                            onClick={() => actOnBrowsedService(svc, 'start')}
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
                            onClick={() => actOnBrowsedService(svc, 'restart')}
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
                            onClick={() => void fetchBrowsedServiceLogs(svc)}
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
                            aria-label={
                              svc.tracked ? 'Unpin service' : 'Pin service'
                            }
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
          </section>
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">
            {filteredBrowseServices.length}/{browseServices.length} services
          </span>
          <ConnectionBadge state={connectionState} />
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

      <Sheet
        open={serviceLogsOpen}
        onOpenChange={(open) => {
          setServiceLogsOpen(open)
          if (!open) {
            setStreamEnabled(false)
            serviceLogsServiceRef.current = null
          }
        }}
      >
        <SheetContent className="flex flex-col gap-0 p-0">
          <SheetHeader className="shrink-0 border-b border-border-subtle px-4 py-3">
            <SheetTitle>{serviceLogsTitle || 'Service logs'}</SheetTitle>
            <SheetDescription>
              {streamEnabled && streamStatus === 'connected' ? (
                <span className="inline-flex items-center gap-1.5">
                  <span className="relative flex h-2 w-2">
                    <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
                    <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
                  </span>
                  Streaming live
                </span>
              ) : (
                'Recent log output'
              )}
            </SheetDescription>
          </SheetHeader>

          <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border-subtle px-4 py-2">
            <div className="relative min-w-0 flex-1">
              <Search className="absolute left-2 top-1.5 h-3.5 w-3.5 text-muted-foreground" />
              <input
                value={serviceLogsSearch}
                onChange={(e) => setServiceLogsSearch(e.target.value)}
                placeholder="Search logs..."
                className={cn(
                  'h-7 w-full rounded-md border border-border-subtle bg-surface-overlay pl-7 text-[11px] placeholder:text-muted-foreground',
                  serviceLogsSearch ? 'pr-7' : 'pr-2',
                )}
              />
              {serviceLogsSearch && (
                <button
                  type="button"
                  className="absolute right-1.5 top-1 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground"
                  onClick={() => setServiceLogsSearch('')}
                  aria-label="Clear search"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
            <TooltipHelper content="Word wrap">
              <Button
                variant={serviceLogsWrap ? 'default' : 'outline'}
                size="sm"
                className="h-7 w-7 cursor-pointer p-0"
                onClick={() => setServiceLogsWrap((v) => !v)}
                aria-label="Toggle word wrap"
              >
                <WrapText className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Refresh logs">
              <Button
                variant="outline"
                size="sm"
                className="h-7 w-7 cursor-pointer p-0"
                onClick={refreshServiceLogs}
                aria-label="Refresh logs"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <TooltipHelper
              content={streamEnabled ? 'Stop streaming' : 'Stream live'}
            >
              <Button
                variant={streamEnabled ? 'default' : 'outline'}
                size="sm"
                className={cn(
                  'h-7 w-7 cursor-pointer p-0',
                  streamEnabled && streamStatus === 'connected'
                    ? 'text-emerald-400'
                    : '',
                )}
                onClick={() => setStreamEnabled((v) => !v)}
                aria-label={
                  streamEnabled ? 'Stop streaming' : 'Stream live'
                }
              >
                <Radio className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Follow output">
              <Button
                variant={serviceLogsFollow ? 'default' : 'outline'}
                size="sm"
                className="h-7 w-7 cursor-pointer p-0"
                onClick={() => setServiceLogsFollow((v) => !v)}
                aria-label="Toggle follow"
              >
                <ArrowDownToLine className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
          </div>

          <LogViewer
            lines={serviceLogLines}
            loading={serviceLogsLoading}
            searchQuery={serviceLogsSearch}
            wordWrap={serviceLogsWrap}
            follow={serviceLogsFollow}
            onFollowChange={setServiceLogsFollow}
            className="min-h-0 flex-1 rounded-none border-0"
          />
        </SheetContent>
      </Sheet>
    </AppShell>
  )
}

export const Route = createFileRoute('/services')({
  component: ServicesPage,
})
