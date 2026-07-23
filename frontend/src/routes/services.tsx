import {
  memo,
  startTransition,
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { RefreshCw, Search, X } from 'lucide-react'
import type {
  OpsBrowseServicesResponse,
  OpsBrowsedService,
  OpsOverviewResponse,
  OpsServiceAction,
  OpsServiceActionResponse,
  OpsServiceInspect,
  OpsServiceStatus,
  OpsServiceStatusResponse,
  OpsServicesResponse,
  OpsUnitActionResponse,
} from '@/types'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import ServicesHelpDialog from '@/components/ServicesHelpDialog'
import { ServiceBrowseRow } from '@/components/services/ServiceBrowseRow'
import { ServiceLogsSheet } from '@/components/services/ServiceLogsSheet'
import { ServicesOperationsSummary } from '@/components/services/ServicesOperationsSummary'
import { ServiceStatusDialog } from '@/components/services/ServiceStatusDialog'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEvents, useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import {
  defaultOpsBrowseUnitTypes,
  deriveOpsTrackedServiceName,
  formatOpsUnitName,
  listOpsBrowseUnitTypes,
  matchesOpsServiceStateFilter,
  matchesOpsServiceTrackFilter,
  sortOpsBrowseUnitTypes,
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'
import type { OpsServiceStateFilter, OpsServiceTrackFilter } from '@/lib/opsServices'
import {
  OPS_BROWSE_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  isOpsWsMessage,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

const EMPTY_SERVICES: Array<OpsServiceStatus> = []
const EMPTY_BROWSE_SERVICES: Array<OpsBrowsedService> = []
const SERVICE_ROW_SKELETON_KEYS = [
  'service-row-system',
  'service-row-user',
  'service-row-timer',
  'service-row-socket',
  'service-row-path',
  'service-row-target',
] as const
const SERVICE_METRIC_SKELETON_KEYS = [
  'service-metric-total',
  'service-metric-running',
  'service-metric-failed',
  'service-metric-tracked',
  'service-metric-actions',
] as const

function browseServiceKey(service: Pick<OpsBrowsedService, 'manager' | 'scope' | 'unit'>): string {
  return `${service.manager}:${service.scope}:${service.unit}`
}

type ServicesBrowseListProps = {
  browseLoading: boolean
  browseServicesCount: number
  browseError: string
  filteredBrowseServices: Array<OpsBrowsedService>
  browsePendingActions: Partial<Record<string, OpsServiceAction>>
  onAction: (svc: OpsBrowsedService, action: OpsServiceAction) => void
  onInspect: (svc: OpsBrowsedService) => void
  onLogs: (svc: OpsBrowsedService) => void
  onToggleTrack: (svc: OpsBrowsedService) => void
  onResetFilters: () => void
  onRefreshBrowse: () => void
}

const ServicesBrowseList = memo(function ServicesBrowseList({
  browseLoading,
  browseServicesCount,
  browseError,
  filteredBrowseServices,
  browsePendingActions,
  onAction,
  onInspect,
  onLogs,
  onToggleTrack,
  onResetFilters,
  onRefreshBrowse,
}: ServicesBrowseListProps) {
  return (
    <ScrollArea className="h-full min-h-0">
      <div className="grid gap-1 p-2">
        {browseLoading &&
          SERVICE_ROW_SKELETON_KEYS.map((key) => (
            <div
              key={key}
              className="h-24 motion-safe:animate-pulse rounded border border-border-subtle bg-surface-elevated"
            />
          ))}
        {filteredBrowseServices.map((svc) => (
          <ServiceBrowseRow
            key={browseServiceKey(svc)}
            service={svc}
            pendingAction={browsePendingActions[browseServiceKey(svc)]}
            onAction={onAction}
            onInspect={onInspect}
            onLogs={onLogs}
            onToggleTrack={onToggleTrack}
          />
        ))}
        {!browseLoading && filteredBrowseServices.length === 0 && (
          <EmptyState variant="inline" className="grid gap-2 p-3 text-[12px]">
            <p>
              {browseServicesCount === 0
                ? 'No units discovered on this host yet.'
                : 'No units match the current filters.'}
            </p>
            <div className="flex flex-wrap gap-2">
              {browseServicesCount > 0 && (
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 cursor-pointer text-[11px]"
                  onClick={onResetFilters}
                >
                  Clear filters
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                className="h-7 cursor-pointer text-[11px]"
                onClick={onRefreshBrowse}
              >
                Refresh discovery
              </Button>
            </div>
          </EmptyState>
        )}
        {browseError !== '' && (
          <div className="grid gap-2 rounded border border-dashed border-destructive/40 bg-destructive/10 p-3 mx-2 mb-2">
            <p className="text-[12px] text-destructive-foreground">{browseError}</p>
            <Button
              variant="outline"
              size="sm"
              className="h-7 w-fit text-[11px]"
              onClick={onRefreshBrowse}
            >
              Try again
            </Button>
          </div>
        )}
      </div>
    </ScrollArea>
  )
})

ServicesBrowseList.displayName = 'ServicesBrowseList'

type ServicesBrowseControlsProps = {
  scopeValue: string
  trackValue: OpsServiceTrackFilter
  searchValue: string
  filteredCount: number
  totalCount: number
  onScopeChange: (value: string) => void
  onTrackChange: (value: OpsServiceTrackFilter) => void
  onSearchChange: (value: string) => void
  onRefreshBrowse: () => void
}

const ServicesBrowseControls = memo(function ServicesBrowseControls({
  scopeValue,
  trackValue,
  searchValue,
  filteredCount,
  totalCount,
  onScopeChange,
  onTrackChange,
  onSearchChange,
  onRefreshBrowse,
}: ServicesBrowseControlsProps) {
  const [searchDraft, setSearchDraft] = useState(searchValue)
  const debouncedSearchDraft = useDebouncedValue(searchDraft)
  const searchValueRef = useRef(searchValue)

  useEffect(() => {
    searchValueRef.current = searchValue
    setSearchDraft(searchValue)
  }, [searchValue])

  useEffect(() => {
    if (debouncedSearchDraft === searchValueRef.current) return
    startTransition(() => {
      onSearchChange(debouncedSearchDraft)
    })
  }, [debouncedSearchDraft, onSearchChange])

  return (
    <div className="flex flex-nowrap items-center gap-2 border-b border-border-subtle p-2 md:flex-wrap">
      <select
        name="services-scope"
        value={scopeValue}
        onChange={(e) => onScopeChange(e.target.value)}
        className="h-9 shrink-0 cursor-pointer rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px] md:h-8"
        aria-label="Filter by scope"
      >
        <option value="all">All scopes</option>
        <option value="user">user</option>
        <option value="system">system</option>
      </select>
      <select
        name="services-tracking"
        value={trackValue}
        onChange={(e) => onTrackChange(e.target.value as OpsServiceTrackFilter)}
        className="hidden h-9 shrink-0 cursor-pointer rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px] md:block md:h-8"
        aria-label="Filter by tracking"
      >
        <option value="all">All pins</option>
        <option value="tracked">Pinned</option>
        <option value="untracked">Unpinned</option>
      </select>
      <div className="flex min-w-0 flex-1 items-center gap-2 md:min-w-44">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground md:top-2" />
          <input
            name="services-search"
            aria-label="Search units"
            value={searchDraft}
            onChange={(e) => setSearchDraft(e.target.value)}
            placeholder="Search units..."
            className={cn(
              'h-9 w-full rounded-md border border-border-subtle bg-surface-overlay pl-8 text-[12px] placeholder:text-muted-foreground md:h-8',
              searchDraft ? 'pr-7' : 'pr-2',
            )}
          />
          {searchDraft && (
            <button
              type="button"
              className="absolute right-1.5 top-2 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground md:top-1.5"
              onClick={() => setSearchDraft('')}
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
            className="h-9 w-9 cursor-pointer text-[11px] md:h-8 md:w-auto"
            onClick={onRefreshBrowse}
          >
            <RefreshCw className="h-3 w-3" />
          </Button>
        </TooltipHelper>
      </div>
      <span className="hidden text-[10px] text-muted-foreground md:inline">
        {filteredCount}/{totalCount} units
      </span>
    </div>
  )
})

ServicesBrowseControls.displayName = 'ServicesBrowseControls'

function ServicesPage() {
  const { tokenRequired, hostname } = useMetaContext()
  const { authenticated } = useTokenContext()
  const { pushToast } = useToastContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [serviceStatusOpen, setServiceStatusOpen] = useState(false)
  const [serviceStatusLoading, setServiceStatusLoading] = useState(false)
  const [serviceStatusError, setServiceStatusError] = useState('')
  const [serviceStatusData, setServiceStatusData] = useState<OpsServiceInspect | null>(null)

  const [serviceLogsOpen, setServiceLogsOpen] = useState(false)
  const [serviceLogsService, setServiceLogsService] = useState<OpsBrowsedService | null>(null)
  const [serviceLogsFetchKey, setServiceLogsFetchKey] = useState(0)

  const [svcStateFilter, setSvcStateFilter] = useState<OpsServiceStateFilter>('all')
  const [svcScopeFilter, setSvcScopeFilter] = useState('all')
  const [svcTrackFilter, setSvcTrackFilter] = useState<OpsServiceTrackFilter>('all')
  const [svcTypeFilter, setSvcTypeFilter] = useState<Array<string>>([])
  const [svcTypeFilterTouched, setSvcTypeFilterTouched] = useState(false)
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
      const data = await api<OpsBrowseServicesResponse>('/api/ops/services/browse')
      return data.services
    },
  })

  const services = servicesQuery.data ?? EMPTY_SERVICES
  const browseServices = browseQuery.data ?? EMPTY_BROWSE_SERVICES
  const browseUnitTypes = useMemo(() => listOpsBrowseUnitTypes(browseServices), [browseServices])
  const defaultSvcTypeFilter = useMemo(
    () => defaultOpsBrowseUnitTypes(browseUnitTypes),
    [browseUnitTypes],
  )
  const effectiveSvcTypeFilter = svcTypeFilterTouched ? svcTypeFilter : defaultSvcTypeFilter
  const allBrowseUnitTypesSelected =
    browseUnitTypes.length > 0 &&
    effectiveSvcTypeFilter.length === browseUnitTypes.length &&
    browseUnitTypes.every((type) => effectiveSvcTypeFilter.includes(type))

  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const browseLoading = browseQuery.isLoading
  const browseError =
    browseQuery.error != null ? toErrorMessage(browseQuery.error, 'failed to browse services') : ''

  const baseFilteredBrowseServices = useMemo(() => {
    let list = browseServices
    if (effectiveSvcTypeFilter.length > 0) {
      list = list.filter((s) => effectiveSvcTypeFilter.includes(s.unitType.trim().toLowerCase()))
    }
    if (svcScopeFilter !== 'all') {
      list = list.filter((s) => s.scope.toLowerCase() === svcScopeFilter.toLowerCase())
    }
    if (svcSearch.trim() !== '') {
      const q = svcSearch.trim().toLowerCase()
      list = list.filter(
        (s) =>
          s.unit.toLowerCase().includes(q) ||
          formatOpsUnitName(s.unit).toLowerCase().includes(q) ||
          s.description.toLowerCase().includes(q),
      )
    }
    return list
  }, [browseServices, effectiveSvcTypeFilter, svcScopeFilter, svcSearch])

  const trackFilteredBrowseServices = useMemo(() => {
    return baseFilteredBrowseServices.filter((service) =>
      matchesOpsServiceTrackFilter(service, svcTrackFilter),
    )
  }, [baseFilteredBrowseServices, svcTrackFilter])

  const filteredBrowseServices = useMemo(() => {
    return trackFilteredBrowseServices.filter((service) =>
      matchesOpsServiceStateFilter(service, svcStateFilter),
    )
  }, [svcStateFilter, trackFilteredBrowseServices])
  const renderedBrowseServices = useDeferredValue(filteredBrowseServices)

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
  const forceReconnectOpsEvents = useOpsEventsReconnect()
  const resyncPage = useCallback(() => {
    forceReconnectOpsEvents()
    refreshPage()
  }, [forceReconnectOpsEvents, refreshPage])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      if (!isOpsWsMessage(message)) return
      switch (message.type) {
        case 'ops.services.updated':
          if (Array.isArray(message.payload.services)) {
            queryClient.setQueryData(OPS_SERVICES_QUERY_KEY, message.payload.services)
          } else {
            void refreshServices()
          }
          void refreshBrowse()
          break
        case 'ops.overview.updated':
          queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, message.payload.overview)
          break
        default:
          break
      }
    },
    [queryClient, refreshBrowse, refreshServices],
  )

  const connectionState = useOpsEvents(handleWSMessage)

  const runServiceAction = useCallback(
    async (serviceName: string, action: OpsServiceAction) => {
      const previous = services.find((item) => item.name === serviceName)
      if (!previous) return

      previousServiceRef.current.set(serviceName, previous)
      queryClient.setQueryData<Array<OpsServiceStatus>>(OPS_SERVICES_QUERY_KEY, (current = []) =>
        current.map((item) =>
          item.name === serviceName ? withOptimisticServiceAction(item, action) : item,
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
      }
    },
    [api, pushToast, queryClient, services],
  )

  const unregisterService = useCallback(
    async (name: string) => {
      const previous = services.find((s) => s.name === name)
      queryClient.setQueryData<Array<OpsServiceStatus>>(OPS_SERVICES_QUERY_KEY, (current = []) =>
        current.filter((service) => service.name !== name),
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
      const key = browseServiceKey(svc)
      setBrowsePendingActions((prev) => ({ ...prev, [key]: action }))
      try {
        if (svc.tracked && svc.trackedName) {
          await runServiceAction(svc.trackedName, action)
        } else {
          const data = await api<OpsUnitActionResponse>('/api/ops/services/unit/action', {
            method: 'POST',
            body: JSON.stringify({
              unit: svc.unit,
              scope: svc.scope,
              manager: svc.manager,
              action,
            }),
          })
          queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, data.overview)
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
          delete next[key]
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
          error instanceof Error ? error.message : 'failed to load service status',
        )
      } finally {
        setServiceStatusLoading(false)
      }
    },
    [api],
  )

  const openServiceLogs = useCallback((svc: OpsBrowsedService) => {
    setServiceLogsService(svc)
    setServiceLogsOpen(true)
    setServiceLogsFetchKey((k) => k + 1)
  }, [])

  const handleLogsOpenChange = useCallback((open: boolean) => {
    setServiceLogsOpen(open)
    if (!open) {
      setServiceLogsService(null)
    }
  }, [])

  const toggleTrack = useCallback(
    async (svc: OpsBrowsedService) => {
      if (svc.tracked && svc.trackedName) {
        await unregisterService(svc.trackedName)
      } else {
        const name = deriveOpsTrackedServiceName(svc.unit)
        try {
          const data = await api<{
            services: Array<OpsServiceStatus>
            globalRev: number
          }>('/api/ops/services', {
            method: 'POST',
            body: JSON.stringify({
              name,
              displayName: svc.description || formatOpsUnitName(svc.unit),
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
            title: svc.description || formatOpsUnitName(svc.unit),
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
  const handleBrowseAction = useCallback(
    (svc: OpsBrowsedService, action: OpsServiceAction) => {
      void actOnBrowsedService(svc, action)
    },
    [actOnBrowsedService],
  )
  const handleInspectBrowsedService = useCallback(
    (svc: OpsBrowsedService) => {
      void inspectBrowsedService(svc)
    },
    [inspectBrowsedService],
  )
  const handleToggleTrack = useCallback(
    (svc: OpsBrowsedService) => {
      void toggleTrack(svc)
    },
    [toggleTrack],
  )

  const toggleStateFilter = useCallback((filter: OpsServiceStateFilter) => {
    setSvcStateFilter((prev) => (prev === filter ? 'all' : filter))
  }, [])

  const toggleTrackFilter = useCallback((filter: OpsServiceTrackFilter) => {
    setSvcTrackFilter((prev) => (prev === filter ? 'all' : filter))
  }, [])

  const toggleTypeFilter = useCallback(
    (unitType: string) => {
      setSvcTypeFilterTouched(true)
      setSvcTypeFilter((prev) => {
        const current = svcTypeFilterTouched ? prev : effectiveSvcTypeFilter
        if (current.includes(unitType)) {
          if (current.length === 1) return current
          return current.filter((item) => item !== unitType)
        }
        return sortOpsBrowseUnitTypes([...current, unitType])
      })
    },
    [effectiveSvcTypeFilter, svcTypeFilterTouched],
  )

  const selectAllTypeFilters = useCallback(() => {
    setSvcTypeFilterTouched(true)
    setSvcTypeFilter(browseUnitTypes)
  }, [browseUnitTypes])

  const resetBrowseFilters = useCallback(() => {
    setSvcSearch('')
    setSvcStateFilter('all')
    setSvcScopeFilter('all')
    setSvcTrackFilter('all')
    setSvcTypeFilter([])
    setSvcTypeFilterTouched(false)
  }, [])

  return (
    <AppShell>
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,var(--section-glow-brand),transparent_34%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <AppSectionTitle hostname={hostname} section="services" />
          </div>
          <div className="flex items-center gap-1.5">
            <ServicesHelpDialog />
            <ConnectionBadge state={connectionState} onClick={resyncPage} />
          </div>
        </header>

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-2 overflow-hidden p-2 md:gap-3 md:p-3">
          <section className="grid gap-2">
            {browseUnitTypes.length > 0 && (
              <div className="no-scrollbar flex min-w-0 flex-nowrap items-center justify-start gap-1 overflow-x-auto rounded-lg border border-border-subtle bg-surface-elevated px-2 py-1.5 md:flex-wrap md:justify-center">
                <button
                  type="button"
                  className={cn(
                    'h-7 shrink-0 cursor-pointer rounded-full border px-2 text-[11px] capitalize transition-colors md:h-6',
                    allBrowseUnitTypesSelected
                      ? 'border-selection-border/50 bg-selection-surface text-selection-foreground'
                      : 'border-border-subtle text-muted-foreground hover:text-foreground',
                  )}
                  onClick={selectAllTypeFilters}
                  aria-pressed={allBrowseUnitTypesSelected}
                >
                  all
                </button>
                {browseUnitTypes.map((unitType) => {
                  const selected = effectiveSvcTypeFilter.includes(unitType)
                  return (
                    <button
                      key={unitType}
                      type="button"
                      className={cn(
                        'h-7 shrink-0 cursor-pointer rounded-full border px-2 text-[11px] capitalize transition-colors md:h-6',
                        selected
                          ? 'border-selection-border/50 bg-selection-surface text-selection-foreground'
                          : 'border-border-subtle text-muted-foreground hover:text-foreground',
                      )}
                      onClick={() => toggleTypeFilter(unitType)}
                      aria-pressed={selected}
                    >
                      {unitType}
                    </button>
                  )
                })}
              </div>
            )}
            {browseLoading ? (
              <div className="grid grid-cols-5 gap-1.5">
                {SERVICE_METRIC_SKELETON_KEYS.map((key) => (
                  <div
                    key={key}
                    className="h-10 motion-safe:animate-pulse rounded-md border border-border-subtle bg-surface-elevated sm:h-12"
                  />
                ))}
              </div>
            ) : (
              <ServicesOperationsSummary
                services={trackFilteredBrowseServices}
                trackingServices={baseFilteredBrowseServices}
                stateFilter={svcStateFilter}
                trackFilter={svcTrackFilter}
                onStateFilterChange={toggleStateFilter}
                onTrackFilterChange={toggleTrackFilter}
              />
            )}
          </section>

          <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ServicesBrowseControls
              scopeValue={svcScopeFilter}
              trackValue={svcTrackFilter}
              searchValue={svcSearch}
              filteredCount={renderedBrowseServices.length}
              totalCount={browseServices.length}
              onScopeChange={setSvcScopeFilter}
              onTrackChange={setSvcTrackFilter}
              onSearchChange={setSvcSearch}
              onRefreshBrowse={refreshBrowse}
            />
            <ServicesBrowseList
              browseLoading={browseLoading}
              browseServicesCount={browseServices.length}
              browseError={browseError}
              filteredBrowseServices={renderedBrowseServices}
              browsePendingActions={browsePendingActions}
              onAction={handleBrowseAction}
              onInspect={handleInspectBrowsedService}
              onLogs={openServiceLogs}
              onToggleTrack={handleToggleTrack}
              onResetFilters={resetBrowseFilters}
              onRefreshBrowse={refreshBrowse}
            />
          </section>
        </div>

        <footer
          aria-live="polite"
          className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground"
        >
          <span className="min-w-0 flex-1 truncate">
            {overviewError !== ''
              ? overviewError
              : `${renderedBrowseServices.length}/${browseServices.length} units`}
          </span>
        </footer>
      </main>

      <ServiceStatusDialog
        open={serviceStatusOpen}
        onOpenChange={setServiceStatusOpen}
        loading={serviceStatusLoading}
        error={serviceStatusError}
        data={serviceStatusData}
      />

      <ServiceLogsSheet
        open={serviceLogsOpen}
        onOpenChange={handleLogsOpenChange}
        fetchKey={serviceLogsFetchKey}
        service={serviceLogsService}
        authenticated={authenticated}
        tokenRequired={tokenRequired}
        api={api}
      />
    </AppShell>
  )
}

export const Route = createFileRoute('/services')({
  component: ServicesPage,
})
