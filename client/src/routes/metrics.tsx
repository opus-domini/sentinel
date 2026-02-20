import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, RefreshCw } from 'lucide-react'
import type {
  OpsHostMetrics,
  OpsMetricsResponse,
  OpsOverviewResponse,
} from '@/types'
import type { MetricsSnapshot } from '@/lib/MetricsHistory'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { TooltipHelper } from '@/components/TooltipHelper'
import MetricsSidebar from '@/components/MetricsSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { MetricCard } from '@/lib/MetricCard'
import { MetricsHistory } from '@/lib/MetricsHistory'
import {
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
  isOpsWsMessage,
} from '@/lib/opsQueryCache'
import { formatBytes, formatUptime, toErrorMessage } from '@/lib/opsUtils'
import { ProgressBar } from '@/lib/ProgressBar'
import { Sparkline } from '@/lib/Sparkline'
import { cn } from '@/lib/utils'

const SPARKLINE_COLORS = {
  cpu: '#10b981',
  memory: '#3b82f6',
  disk: '#f59e0b',
  loadAvg: '#8b5cf6',
  goroutines: '#06b6d4',
  goHeap: '#f97316',
} as const

const round1 = (n: number) => Math.round(n * 10) / 10

function toSnapshot(m: OpsHostMetrics): MetricsSnapshot {
  return {
    cpuPercent: round1(m.cpuPercent),
    memPercent: round1(m.memPercent),
    diskPercent: round1(m.diskPercent),
    loadAvg1: m.loadAvg1,
    numGoroutines: m.numGoroutines,
    goMemAllocMB: m.goMemAllocMB,
  }
}

function formatCollectedAt(value: string): string {
  const parsed = Date.parse(value)
  if (Number.isNaN(parsed)) {
    return '-'
  }
  return new Date(parsed).toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

type MetricsFooterSummaryParams = {
  overviewError: string
  metricsError: string
  overviewLoading: boolean
  metricsLoading: boolean
  collectedAt: string
}

function buildMetricsFooterSummary({
  overviewError,
  metricsError,
  overviewLoading,
  metricsLoading,
  collectedAt,
}: MetricsFooterSummaryParams): string {
  if (overviewError.trim() !== '') {
    return overviewError
  }
  if (metricsError.trim() !== '') {
    return metricsError
  }
  if (overviewLoading || metricsLoading) {
    return 'Loading metrics...'
  }
  return `Sample ${formatCollectedAt(collectedAt)}`
}

type MetricsTab = 'system' | 'runtime'

function MetricsPage() {
  const { tokenRequired } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const layout = useLayoutContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<MetricsTab>('system')
  const historyRef = useRef(new MetricsHistory())
  const seededRef = useRef(false)

  const overviewQuery = useQuery({
    queryKey: OPS_OVERVIEW_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsOverviewResponse>('/api/ops/overview')
      return data.overview
    },
  })

  const metricsQuery = useQuery({
    queryKey: OPS_METRICS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsMetricsResponse>('/api/ops/metrics')
      return data.metrics
    },
  })

  const overview = overviewQuery.data ?? null
  const metrics = metricsQuery.data ?? null
  const overviewLoading = overviewQuery.isLoading
  const metricsLoading = metricsQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const metricsError =
    metricsQuery.error != null
      ? toErrorMessage(metricsQuery.error, 'failed to load metrics')
      : ''

  useEffect(() => {
    if (metrics != null && !seededRef.current) {
      historyRef.current.push(toSnapshot(metrics))
      seededRef.current = true
    }
  }, [metrics])

  const refreshOverview = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_OVERVIEW_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshMetrics = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_METRICS_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshPage = useCallback(() => {
    void refreshOverview()
    void refreshMetrics()
  }, [refreshOverview, refreshMetrics])

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
        case 'ops.metrics.updated': {
          const m = message.payload.metrics
          historyRef.current.push(toSnapshot(m))
          queryClient.setQueryData(OPS_METRICS_QUERY_KEY, m)
          break
        }
        default:
          break
      }
    },
    [queryClient],
  )

  const connectionState = useOpsEventsSocket({
    authenticated,
    tokenRequired,
    onMessage: handleWSMessage,
  })
  const footerSummary = buildMetricsFooterSummary({
    overviewError,
    metricsError,
    overviewLoading,
    metricsLoading,
    collectedAt: metrics?.collectedAt ?? '',
  })
  const footerCadence = metrics != null ? 'Live · 2s' : 'waiting'

  const history = historyRef.current
  const cpuData = history.field('cpuPercent')
  const memData = history.field('memPercent')
  const diskData = history.field('diskPercent')
  const loadData = history.field('loadAvg1')
  const goroutineData = history.field('numGoroutines')
  const heapData = history.field('goMemAllocMB')
  const ts = history.timestamps()

  return (
    <AppShell
      sidebar={
        <MetricsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          overview={overview}
          onTokenChange={setToken}
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
            <span className="truncate text-muted-foreground">metrics</span>
          </div>
          <div className="flex items-center gap-1.5">
            <TooltipHelper content="Refresh">
              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6 cursor-pointer"
                onClick={refreshPage}
                aria-label="Refresh metrics"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden p-3">
          <div className="flex gap-1" role="tablist">
            {(['system', 'runtime'] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={activeTab === tab}
                onClick={() => setActiveTab(tab)}
                className={cn(
                  'rounded-md border px-3 py-1 text-[11px] font-medium capitalize transition-colors',
                  activeTab === tab
                    ? 'border-foreground/20 bg-foreground/10 text-foreground'
                    : 'border-border-subtle bg-surface-elevated text-muted-foreground hover:text-foreground',
                )}
              >
                {tab}
              </button>
            ))}
          </div>

          <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-3 p-2">
                {metricsLoading && (
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    {Array.from({ length: activeTab === 'system' ? 4 : 4 }).map(
                      (_, idx) => (
                        <div
                          key={`metrics-skeleton-${idx}`}
                          className="h-28 animate-pulse rounded-lg border border-border-subtle bg-surface-elevated"
                        />
                      ),
                    )}
                  </div>
                )}
                {metricsError !== '' && (
                  <div className="grid gap-2 rounded border border-dashed border-destructive/40 bg-destructive/10 p-3">
                    <p className="text-[12px] text-destructive-foreground">
                      {metricsError}
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
                {!metricsLoading && metrics == null && metricsError === '' && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>No metric sample received yet.</p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-fit text-[11px]"
                      onClick={refreshPage}
                    >
                      Refresh metrics
                    </Button>
                  </div>
                )}
                {!metricsLoading &&
                  metrics != null &&
                  activeTab === 'system' && (
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <div
                        className={cn(
                          'rounded-lg border p-2.5',
                          metrics.cpuPercent > 90
                            ? 'border-red-500/40 bg-red-500/10'
                            : 'border-border-subtle bg-surface-elevated',
                        )}
                      >
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          CPU
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.cpuPercent >= 0
                            ? `${metrics.cpuPercent.toFixed(1)}%`
                            : '-'}
                        </p>
                        <ProgressBar
                          percent={
                            metrics.cpuPercent >= 0 ? metrics.cpuPercent : 0
                          }
                        />
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={cpuData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.cpu}
                            domain={[0, 100]}
                            formatValue={(v) => `${v.toFixed(1)}%`}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                      <div
                        className={cn(
                          'rounded-lg border p-2.5',
                          metrics.memPercent > 90
                            ? 'border-red-500/40 bg-red-500/10'
                            : 'border-border-subtle bg-surface-elevated',
                        )}
                      >
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          Memory
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.memPercent.toFixed(1)}%
                        </p>
                        <p className="text-[10px] text-muted-foreground">
                          {formatBytes(metrics.memUsedBytes)} /{' '}
                          {formatBytes(metrics.memTotalBytes)}
                        </p>
                        <ProgressBar percent={metrics.memPercent} />
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={memData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.memory}
                            domain={[0, 100]}
                            formatValue={(v) => `${v.toFixed(1)}%`}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                      <div
                        className={cn(
                          'rounded-lg border p-2.5',
                          metrics.diskPercent > 95
                            ? 'border-red-500/40 bg-red-500/10'
                            : 'border-border-subtle bg-surface-elevated',
                        )}
                      >
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          Disk
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.diskPercent.toFixed(1)}%
                        </p>
                        <p className="text-[10px] text-muted-foreground">
                          {formatBytes(metrics.diskUsedBytes)} /{' '}
                          {formatBytes(metrics.diskTotalBytes)}
                        </p>
                        <ProgressBar percent={metrics.diskPercent} />
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={diskData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.disk}
                            domain={[0, 100]}
                            formatValue={(v) => `${v.toFixed(1)}%`}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                      <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          Load Avg
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.loadAvg1.toFixed(2)}
                        </p>
                        <p className="text-[10px] text-muted-foreground">
                          {metrics.loadAvg5.toFixed(2)} /{' '}
                          {metrics.loadAvg15.toFixed(2)}
                        </p>
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={loadData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.loadAvg}
                            formatValue={(v) => v.toFixed(2)}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                    </div>
                  )}
                {!metricsLoading &&
                  metrics != null &&
                  activeTab === 'runtime' && (
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          Goroutines
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.numGoroutines}
                        </p>
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={goroutineData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.goroutines}
                            formatValue={(v) => `${v}`}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                      <div className="rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
                        <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                          Go Heap
                        </p>
                        <p className="mt-1 text-[12px] font-semibold">
                          {metrics.goMemAllocMB.toFixed(1)} MB
                        </p>
                        <div className="mt-2 h-10 w-full">
                          <Sparkline
                            data={heapData}
                            timestamps={ts}
                            color={SPARKLINE_COLORS.goHeap}
                            formatValue={(v) => `${v.toFixed(1)} MB`}
                            className="h-full w-full"
                          />
                        </div>
                      </div>
                      <MetricCard
                        label="PID"
                        value={`${overview?.sentinel.pid ?? '-'}`}
                      />
                      <MetricCard
                        label="Uptime"
                        value={
                          overview != null
                            ? formatUptime(overview.sentinel.uptimeSec)
                            : '-'
                        }
                      />
                    </div>
                  )}
              </div>
            </ScrollArea>
          </section>
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">{footerSummary}</span>
          <span className="shrink-0 whitespace-nowrap">{footerCadence}</span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/metrics')({
  component: MetricsPage,
})
