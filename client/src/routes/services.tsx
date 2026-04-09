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
import {
  AlertTriangle,
  CheckCircle2,
  CircleOff,
  Layers,
  Menu,
  RefreshCw,
  Search,
  X,
} from 'lucide-react'
import type {
  OpsActivityEvent,
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
import { ServiceBrowseRow } from '@/components/services/ServiceBrowseRow'
import { ServiceLogsSheet } from '@/components/services/ServiceLogsSheet'
import { ServiceStatusDialog } from '@/components/services/ServiceStatusDialog'
import ServicesSidebar from '@/components/ServicesSidebar'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import {
  defaultOpsBrowseUnitTypes,
  deriveOpsTrackedServiceName,
  listOpsBrowseUnitTypes,
  sortOpsBrowseUnitTypes,
  upsertOpsService,
  withOptimisticServiceAction,
} from '@/lib/opsServices'
import { MetricCard } from '@/lib/MetricCard'
import {
  OPS_ALERTS_QUERY_KEY,
  OPS_BROWSE_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  OPS_SERVICES_QUERY_KEY,
  isOpsWsMessage,
  opsActivityQueryKey,
  prependOpsActivityEvent,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

function browseServiceKey(
  service: Pick<OpsBrowsedService, 'manager' | 'scope' | 'unit'>,
): string {
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
          Array.from({ length: 6 }).map((_, idx) => (
            <div
              key={`svc-row-skeleton-${idx}`}
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
            <p className="text-[12px] text-destructive-foreground">
              {browseError}
            </p>
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
  searchValue: string
  filteredCount: number
  totalCount: number
  onScopeChange: (value: string) => void
  onSearchChange: (value: string) => void
  onRefreshBrowse: () => void
}

const ServicesBrowseControls = memo(function ServicesBrowseControls({
  scopeValue,
  searchValue,
  filteredCount,
  totalCount,
  onScopeChange,
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
    <div className="flex flex-wrap items-center gap-2 border-b border-border-subtle p-2">
      <select
        value={scopeValue}
        onChange={(e) => onScopeChange(e.target.value)}
        className="h-7 flex-1 cursor-pointer rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px] md:h-8 md:flex-none"
        aria-label="Filter by scope"
      >
        <option value="all">All scopes</option>
        <option value="user">user</option>
        <option value="system">system</option>
      </select>
      <div className="flex w-full items-center gap-2 md:w-auto md:min-w-44 md:flex-1">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-2 top-1.5 h-4 w-4 text-muted-foreground md:top-2" />
          <input
            value={searchDraft}
            onChange={(e) => setSearchDraft(e.target.value)}
            placeholder="Search units..."
            className={cn(
              'h-7 w-full rounded-md border border-border-subtle bg-surface-overlay pl-8 text-[12px] placeholder:text-muted-foreground md:h-8',
              searchDraft ? 'pr-7' : 'pr-2',
            )}
          />
          {searchDraft && (
            <button
              type="button"
              className="absolute right-1.5 top-1 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground md:top-1.5"
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
            className="h-7 cursor-pointer text-[11px] md:h-8"
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
  const { authenticated, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [serviceStatusOpen, setServiceStatusOpen] = useState(false)
  const [serviceStatusLoading, setServiceStatusLoading] = useState(false)
  const [serviceStatusError, setServiceStatusError] = useState('')
  const [serviceStatusData, setServiceStatusData] =
    useState<OpsServiceInspect | null>(null)

  const [serviceLogsOpen, setServiceLogsOpen] = useState(false)
  const [serviceLogsService, setServiceLogsService] =
    useState<OpsBrowsedService | null>(null)
  const [serviceLogsFetchKey, setServiceLogsFetchKey] = useState(0)

  const [svcStateFilter, setSvcStateFilter] = useState('all')
  const [svcScopeFilter, setSvcScopeFilter] = useState('all')
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
      const data = await api<OpsBrowseServicesResponse>(
        '/api/ops/services/browse',
      )
      return data.services
    },
  })

  const services = servicesQuery.data ?? []
  const browseServices = browseQuery.data ?? []
  const browseUnitTypes = useMemo(
    () => listOpsBrowseUnitTypes(browseServices),
    [browseServices],
  )
  const defaultSvcTypeFilter = useMemo(
    () => defaultOpsBrowseUnitTypes(browseUnitTypes),
    [browseUnitTypes],
  )
  const effectiveSvcTypeFilter = svcTypeFilterTouched
    ? svcTypeFilter
    : defaultSvcTypeFilter
  const allBrowseUnitTypesSelected =
    browseUnitTypes.length > 0 &&
    effectiveSvcTypeFilter.length === browseUnitTypes.length &&
    browseUnitTypes.every((type) => effectiveSvcTypeFilter.includes(type))

  const servicesLoading = servicesQuery.isLoading
  const servicesError =
    servicesQuery.error != null
      ? toErrorMessage(servicesQuery.error, 'failed to load services')
      : ''
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const browseLoading = browseQuery.isLoading
  const browseError =
    browseQuery.error != null
      ? toErrorMessage(browseQuery.error, 'failed to browse services')
      : ''

  const baseFilteredBrowseServices = useMemo(() => {
    let list = browseServices
    if (effectiveSvcTypeFilter.length > 0) {
      list = list.filter((s) =>
        effectiveSvcTypeFilter.includes(s.unitType.trim().toLowerCase()),
      )
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
  }, [browseServices, effectiveSvcTypeFilter, svcScopeFilter, svcSearch])

  const filteredBrowseServices = useMemo(() => {
    let list = baseFilteredBrowseServices
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
    return list
  }, [baseFilteredBrowseServices, svcStateFilter])
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

  const handleWSMessage = useCallback(
    (message: unknown) => {
      if (!isOpsWsMessage(message)) return
      switch (message.type) {
        case 'ops.services.updated':
          if (Array.isArray(message.payload.services)) {
            queryClient.setQueryData(
              OPS_SERVICES_QUERY_KEY,
              message.payload.services,
            )
          } else {
            void refreshServices()
          }
          void refreshBrowse()
          break
        case 'ops.overview.updated':
          queryClient.setQueryData(
            OPS_OVERVIEW_QUERY_KEY,
            message.payload.overview,
          )
          break
        default:
          break
      }
    },
    [queryClient, refreshBrowse, refreshServices],
  )

  const connectionState = useOpsEventsSocket({
    authenticated,
    tokenRequired,
    onMessage: handleWSMessage,
  })

  const runServiceAction = useCallback(
    async (serviceName: string, action: OpsServiceAction) => {
      const previous = services.find((item) => item.name === serviceName)
      if (!previous) return

      previousServiceRef.current.set(serviceName, previous)
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
          queryClient.setQueryData<Array<OpsActivityEvent>>(
            opsActivityQueryKey('', 'all'),
            (current = []) =>
              prependOpsActivityEvent(
                current,
                data.timelineEvent as OpsActivityEvent,
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
      const key = browseServiceKey(svc)
      setBrowsePendingActions((prev) => ({ ...prev, [key]: action }))
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
            queryClient.setQueryData<Array<OpsActivityEvent>>(
              opsActivityQueryKey('', 'all'),
              (current = []) =>
                prependOpsActivityEvent(
                  current,
                  data.timelineEvent as OpsActivityEvent,
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

  const navigateToService = useCallback(
    (unit: string) => {
      setSvcStateFilter('all')
      setSvcScopeFilter('all')
      const matchingTypes = listOpsBrowseUnitTypes(
        browseServices.filter((service) => service.unit === unit),
      )
      if (matchingTypes.length > 0) {
        setSvcTypeFilterTouched(true)
        setSvcTypeFilter(matchingTypes)
      } else {
        setSvcTypeFilter([])
        setSvcTypeFilterTouched(false)
      }
      setSvcSearch(unit)
      layout.setSidebarOpen(false)
    },
    [browseServices, layout],
  )

  const stats = useMemo(() => {
    const list = baseFilteredBrowseServices
    const total = list.length
    let active = 0
    let inactive = 0
    let failed = 0
    for (const s of list) {
      const state = s.activeState.trim().toLowerCase()
      if (state === 'active' || state === 'running') active++
      else if (state === 'failed') failed++
      else if (state === 'inactive' || state === 'dead') inactive++
    }
    return {
      total: `${total}`,
      active: `${active}`,
      inactive: `${inactive}`,
      failed: `${failed}`,
    }
  }, [baseFilteredBrowseServices])

  const toggleStateFilter = useCallback((filter: string) => {
    setSvcStateFilter((prev) => (prev === filter ? 'all' : filter))
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
    setSvcTypeFilter([])
    setSvcTypeFilterTouched(false)
  }, [])

  return (
    <AppShell
      sidebar={
        <ServicesSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
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
              className="cursor-pointer md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <AppSectionTitle hostname={hostname} section="services" />
          </div>
          <div className="flex items-center gap-1.5">
            <TooltipHelper content="Refresh">
              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6 cursor-pointer"
                onClick={refreshPage}
                aria-label="Refresh services"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-2 overflow-hidden p-2 md:gap-3 md:p-3">
          <section className="grid gap-2">
            {browseUnitTypes.length > 0 && (
              <div className="flex min-w-0 flex-wrap items-center justify-center gap-1 rounded-lg border border-border-subtle bg-surface-elevated px-2 py-1.5">
                <button
                  type="button"
                  className={cn(
                    'h-6 cursor-pointer rounded-full border px-2 text-[11px] capitalize transition-colors',
                    allBrowseUnitTypesSelected
                      ? 'border-cyan-500/40 bg-cyan-500/10 text-foreground'
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
                        'h-6 cursor-pointer rounded-full border px-2 text-[11px] capitalize transition-colors',
                        selected
                          ? 'border-cyan-500/40 bg-cyan-500/10 text-foreground'
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
              <>
                <div className="hidden gap-2 md:grid md:grid-cols-4">
                  {Array.from({ length: 4 }).map((_, idx) => (
                    <div
                      key={`svc-metric-skeleton-${idx}`}
                      className="h-20 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-surface-elevated"
                    />
                  ))}
                </div>
                <div className="h-9 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-surface-elevated md:hidden" />
              </>
            ) : (
              <>
                <div className="hidden gap-2 md:grid md:grid-cols-4">
                  <MetricCard
                    label="Total"
                    value={stats.total}
                    onClick={() => setSvcStateFilter('all')}
                    selected={svcStateFilter === 'all'}
                  />
                  <MetricCard
                    label="Active"
                    value={stats.active}
                    onClick={() => toggleStateFilter('active')}
                    selected={svcStateFilter === 'active'}
                  />
                  <MetricCard
                    label="Inactive"
                    value={stats.inactive}
                    onClick={() => toggleStateFilter('inactive')}
                    selected={svcStateFilter === 'inactive'}
                  />
                  <MetricCard
                    label="Failed"
                    value={stats.failed}
                    alert={Number(stats.failed) > 0}
                    onClick={() => toggleStateFilter('failed')}
                    selected={svcStateFilter === 'failed'}
                  />
                </div>
                <div className="flex items-center justify-center gap-1 md:hidden">
                  <button
                    type="button"
                    className={cn(
                      'flex h-6 cursor-pointer items-center gap-1 rounded-full border px-2 text-[11px] transition-colors',
                      svcStateFilter === 'all'
                        ? 'border-cyan-500/40 bg-cyan-500/10 text-foreground'
                        : 'border-border-subtle text-muted-foreground hover:text-foreground',
                    )}
                    onClick={() => setSvcStateFilter('all')}
                    aria-pressed={svcStateFilter === 'all'}
                  >
                    <Layers className="h-3 w-3" />
                    <span className="font-semibold">{stats.total}</span>
                  </button>
                  <button
                    type="button"
                    className={cn(
                      'flex h-6 cursor-pointer items-center gap-1 rounded-full border px-2 text-[11px] transition-colors',
                      svcStateFilter === 'active'
                        ? 'border-ok/40 bg-ok/10 text-ok-foreground'
                        : 'border-border-subtle text-muted-foreground hover:text-foreground',
                    )}
                    onClick={() => toggleStateFilter('active')}
                    aria-pressed={svcStateFilter === 'active'}
                  >
                    <CheckCircle2 className="h-3 w-3" />
                    <span className="font-semibold">{stats.active}</span>
                  </button>
                  <button
                    type="button"
                    className={cn(
                      'flex h-6 cursor-pointer items-center gap-1 rounded-full border px-2 text-[11px] transition-colors',
                      svcStateFilter === 'inactive'
                        ? 'border-cyan-500/40 bg-cyan-500/10 text-foreground'
                        : 'border-border-subtle text-muted-foreground hover:text-foreground',
                    )}
                    onClick={() => toggleStateFilter('inactive')}
                    aria-pressed={svcStateFilter === 'inactive'}
                  >
                    <CircleOff className="h-3 w-3" />
                    <span className="font-semibold">{stats.inactive}</span>
                  </button>
                  <button
                    type="button"
                    className={cn(
                      'flex h-6 cursor-pointer items-center gap-1 rounded-full border px-2 text-[11px] transition-colors',
                      svcStateFilter === 'failed'
                        ? 'border-destructive/40 bg-destructive/10 text-destructive-foreground'
                        : Number(stats.failed) > 0
                          ? 'border-destructive/30 text-destructive-foreground hover:text-destructive-foreground'
                          : 'border-border-subtle text-muted-foreground hover:text-foreground',
                    )}
                    onClick={() => toggleStateFilter('failed')}
                    aria-pressed={svcStateFilter === 'failed'}
                  >
                    <AlertTriangle className="h-3 w-3" />
                    <span className="font-semibold">{stats.failed}</span>
                  </button>
                </div>
              </>
            )}
          </section>

          <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ServicesBrowseControls
              scopeValue={svcScopeFilter}
              searchValue={svcSearch}
              filteredCount={renderedBrowseServices.length}
              totalCount={browseServices.length}
              onScopeChange={setSvcScopeFilter}
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
