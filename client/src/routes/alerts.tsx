import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Eye, EyeOff, Menu, RefreshCw, Trash2 } from 'lucide-react'
import type {
  OpsActivityEvent,
  OpsAlert,
  OpsAlertsResponse,
  OpsOverviewResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import AlertsSidebar from '@/components/AlertsSidebar'
import ConnectionBadge from '@/components/ConnectionBadge'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_ALERTS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  isOpsWsMessage,
  opsActivityQueryKey,
  prependOpsActivityEvent,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

function formatAlertTime(isoDate: string): string {
  const parsed = Date.parse(isoDate)
  if (Number.isNaN(parsed)) return isoDate
  const d = new Date(parsed)
  const now = Date.now()
  const diffMin = Math.floor((now - d.getTime()) / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  return d.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

type AlertsFooterSummaryParams = {
  overviewError: string
  alertsError: string
  overviewLoading: boolean
  alertsLoading: boolean
  filteredCount: number
  totalCount: number
  openCount: number
}

function buildAlertsFooterSummary({
  overviewError,
  alertsError,
  overviewLoading,
  alertsLoading,
  filteredCount,
  totalCount,
  openCount,
}: AlertsFooterSummaryParams): string {
  if (overviewError.trim() !== '') {
    return overviewError
  }
  if (alertsError.trim() !== '') {
    return alertsError
  }
  if (overviewLoading || alertsLoading) {
    return 'Loading alerts...'
  }
  return `${filteredCount}/${totalCount} alerts · ${openCount} open`
}

function AlertsPage() {
  const { tokenRequired } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [selectedSeverity, setSelectedSeverity] = useState('all')
  const [showResolved, setShowResolved] = useState(false)
  const [confirmDismissId, setConfirmDismissId] = useState<number | null>(null)

  useEffect(() => {
    if (confirmDismissId == null) return
    const timer = setTimeout(() => setConfirmDismissId(null), 3000)
    return () => clearTimeout(timer)
  }, [confirmDismissId])

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

  const overview = overviewQuery.data ?? null
  const alerts = alertsQuery.data ?? []
  const overviewLoading = overviewQuery.isLoading
  const alertsLoading = alertsQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const alertsError =
    alertsQuery.error != null
      ? toErrorMessage(alertsQuery.error, 'failed to load alerts')
      : ''

  const filteredAlerts = useMemo(() => {
    let result = alerts
    if (!showResolved) {
      result = result.filter((a) => a.status !== 'resolved')
    }
    if (selectedSeverity !== 'all') {
      result = result.filter((a) => a.severity === selectedSeverity)
    }
    return result
  }, [alerts, selectedSeverity, showResolved])

  const openCount = useMemo(
    () => alerts.filter((a) => a.status === 'open').length,
    [alerts],
  )

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

  const refreshPage = useCallback(() => {
    void refreshOverview()
    void refreshAlerts()
  }, [refreshOverview, refreshAlerts])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      if (!isOpsWsMessage(message)) return
      switch (message.type) {
        case 'ops.overview.updated':
          queryClient.setQueryData(
            OPS_OVERVIEW_QUERY_KEY,
            message.payload.overview,
          )
          break
        case 'ops.alerts.updated':
          if (Array.isArray(message.payload.alerts)) {
            queryClient.setQueryData(
              OPS_ALERTS_QUERY_KEY,
              message.payload.alerts,
            )
          } else {
            void refreshAlerts()
          }
          break
        default:
          break
      }
    },
    [queryClient, refreshAlerts],
  )

  const connectionState = useOpsEventsSocket({
    authenticated,
    tokenRequired,
    onMessage: handleWSMessage,
  })
  const footerSummary = buildAlertsFooterSummary({
    overviewError,
    alertsError,
    overviewLoading,
    alertsLoading,
    filteredCount: filteredAlerts.length,
    totalCount: alerts.length,
    openCount,
  })
  const footerCadence = alertsQuery.isSuccess ? 'Live · 5s' : 'waiting'

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
          timelineEvent?: OpsActivityEvent
        }>(`/api/ops/alerts/${alertID}/ack`, {
          method: 'POST',
        })
        queryClient.setQueryData<Array<OpsAlert>>(
          OPS_ALERTS_QUERY_KEY,
          (current = []) =>
            current.map((item) => (item.id === alertID ? data.alert : item)),
        )
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

  const dismissAlert = useCallback(
    async (alertID: number) => {
      const previous = alerts.find((item) => item.id === alertID)
      if (!previous) return

      queryClient.setQueryData<Array<OpsAlert>>(
        OPS_ALERTS_QUERY_KEY,
        (current = []) => current.filter((item) => item.id !== alertID),
      )

      try {
        await api(`/api/ops/alerts/${alertID}`, {
          method: 'DELETE',
        })
      } catch (error) {
        queryClient.setQueryData<Array<OpsAlert>>(
          OPS_ALERTS_QUERY_KEY,
          (current = []) => [...current, previous],
        )
        pushToast({
          level: 'error',
          title: previous.title,
          message:
            error instanceof Error ? error.message : 'failed to dismiss alert',
        })
      }
    },
    [alerts, api, pushToast, queryClient],
  )

  return (
    <AppShell
      sidebar={
        <AlertsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          alertCount={alerts.length}
          openCount={openCount}
          overview={overview}
          selectedSeverity={selectedSeverity}
          onSeverityChange={setSelectedSeverity}
          onTokenChange={setToken}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(245,158,11,.16),transparent_34%),var(--background)]">
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
            <span className="truncate text-muted-foreground">alerts</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={() => setShowResolved((prev) => !prev)}
              aria-label={showResolved ? 'Hide resolved' : 'Show resolved'}
            >
              {showResolved ? (
                <Eye className="h-3.5 w-3.5" />
              ) : (
                <EyeOff className="h-3.5 w-3.5" />
              )}
              <span className="hidden md:inline">Resolved</span>
            </Button>
            <TooltipHelper content="Refresh">
              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6 cursor-pointer"
                onClick={refreshPage}
                aria-label="Refresh alerts"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden p-3">
          <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            {!alertsLoading &&
            filteredAlerts.length === 0 &&
            alertsError === '' ? (
              <div className="grid h-full place-items-center">
                <div className="text-center">
                  <p className="text-[13px] text-muted-foreground">
                    {selectedSeverity === 'all'
                      ? 'No active alerts.'
                      : `No ${selectedSeverity} alerts.`}
                  </p>
                  <div className="mt-3 flex flex-wrap justify-center gap-2">
                    {selectedSeverity !== 'all' && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 text-[11px]"
                        onClick={() => setSelectedSeverity('all')}
                      >
                        Show all severities
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 text-[11px]"
                      onClick={refreshPage}
                    >
                      Refresh alerts
                    </Button>
                  </div>
                </div>
              </div>
            ) : (
              <ScrollArea className="h-full min-h-0">
                <div className="grid gap-1.5 p-2">
                  {alertsLoading &&
                    Array.from({ length: 5 }).map((_, idx) => (
                      <div
                        key={`alerts-skeleton-${idx}`}
                        className="h-24 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                      />
                    ))}
                  {filteredAlerts.map((alert) => (
                    <div
                      key={alert.id}
                      className={cn(
                        'grid gap-1.5 rounded border px-2.5 py-2',
                        alert.status === 'resolved'
                          ? 'border-border-subtle bg-surface-elevated opacity-60'
                          : alert.severity === 'error'
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
                        <div className="flex shrink-0 items-center gap-1">
                          <span className="rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                            {alert.status}
                          </span>
                          {alert.status === 'resolved' &&
                            (confirmDismissId === alert.id ? (
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-6 px-1.5 text-[10px] text-destructive-foreground"
                                onClick={() => {
                                  setConfirmDismissId(null)
                                  void dismissAlert(alert.id)
                                }}
                              >
                                Confirm?
                              </Button>
                            ) : (
                              <TooltipHelper content="Dismiss">
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="h-6 w-6 text-muted-foreground"
                                  onClick={() => setConfirmDismissId(alert.id)}
                                  aria-label="Dismiss alert"
                                >
                                  <Trash2 className="h-3 w-3" />
                                </Button>
                              </TooltipHelper>
                            ))}
                        </div>
                      </div>
                      <p className="text-[11px] text-muted-foreground">
                        {alert.message}
                      </p>
                      <p className="text-[10px] text-muted-foreground">
                        {formatAlertTime(alert.firstSeenAt)}
                        {alert.lastSeenAt !== alert.firstSeenAt &&
                          ` · last ${formatAlertTime(alert.lastSeenAt)}`}
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
                  {alertsError !== '' && (
                    <div className="grid gap-2 rounded border border-dashed border-destructive/40 bg-destructive/10 p-3">
                      <p className="text-[12px] text-destructive-foreground">
                        {alertsError}
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 w-fit text-[11px]"
                        onClick={refreshPage}
                      >
                        Try again
                      </Button>
                    </div>
                  )}
                </div>
              </ScrollArea>
            )}
          </div>
        </section>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">{footerSummary}</span>
          <span className="shrink-0 whitespace-nowrap">{footerCadence}</span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/alerts')({
  component: AlertsPage,
})
