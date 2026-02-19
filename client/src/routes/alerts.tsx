import { useCallback, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, RefreshCw } from 'lucide-react'
import type {
  OpsAlert,
  OpsAlertsResponse,
  OpsOverviewResponse,
  OpsTimelineEvent,
  OpsWsMessage,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import AlertsSidebar from '@/components/AlertsSidebar'
import ConnectionBadge from '@/components/ConnectionBadge'
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
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

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
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [selectedSeverity, setSelectedSeverity] = useState('all')

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
    if (selectedSeverity === 'all') return alerts
    return alerts.filter((a) => a.severity === selectedSeverity)
  }, [alerts, selectedSeverity])

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
      const msg = message as OpsWsMessage
      switch (msg.type) {
        case 'ops.overview.updated':
          queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, msg.payload.overview)
          break
        case 'ops.alerts.updated':
          if (Array.isArray(msg.payload.alerts)) {
            queryClient.setQueryData(OPS_ALERTS_QUERY_KEY, msg.payload.alerts)
          } else {
            void refreshAlerts()
          }
          break
        default:
          break
      }
    },
    [queryClient, refreshOverview, refreshAlerts],
  )

  const connectionState = useOpsEventsSocket({
    token,
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
            opsTimelineQueryKey('', 'all'),
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

  return (
    <AppShell
      sidebar={
        <AlertsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
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
              onClick={refreshPage}
              aria-label="Refresh alerts"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden p-3">
          <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
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
                {!alertsLoading && filteredAlerts.length === 0 && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>
                      {selectedSeverity === 'all'
                        ? 'No active alerts.'
                        : `No ${selectedSeverity} alerts.`}
                    </p>
                    <div className="flex flex-wrap gap-2">
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
                )}
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
