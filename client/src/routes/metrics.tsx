import { useCallback, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, RefreshCw } from 'lucide-react'
import type {
  OpsHostMetrics,
  OpsMetricsResponse,
  OpsOverview,
  OpsOverviewResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import MetricsSidebar from '@/components/MetricsSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { MetricCard } from '@/lib/MetricCard'
import {
  OPS_METRICS_QUERY_KEY,
  OPS_OVERVIEW_QUERY_KEY,
} from '@/lib/opsQueryCache'
import {
  formatBytes,
  formatTimeAgo,
  formatUptime,
  toErrorMessage,
} from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(percent, 100)
  const color =
    percent > 90
      ? 'bg-red-500'
      : percent > 80
        ? 'bg-amber-500'
        : 'bg-emerald-500'

  return (
    <div className="mt-1.5 h-1.5 w-full rounded-full bg-surface-overlay">
      <div
        className={cn('h-1.5 rounded-full', color)}
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}

type MetricsTab = 'system' | 'runtime'

function MetricsPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<MetricsTab>('system')

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
      const typed = message as {
        type?: string
        payload?: { overview?: OpsOverview; metrics?: OpsHostMetrics }
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
        case 'ops.metrics.updated':
          if (
            typed.payload?.metrics != null &&
            typeof typed.payload.metrics === 'object'
          ) {
            queryClient.setQueryData(
              OPS_METRICS_QUERY_KEY,
              typed.payload.metrics,
            )
          }
          break
        default:
          break
      }
    },
    [queryClient, refreshOverview],
  )

  const connectionState = useOpsEventsSocket({
    token,
    tokenRequired,
    onMessage: handleWSMessage,
  })

  return (
    <AppShell
      sidebar={
        <MetricsSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          overview={overview}
          metrics={metrics}
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
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh metrics"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <div className="grid min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden p-3">
          <div className="flex gap-1">
            {(['system', 'runtime'] as const).map((tab) => (
              <button
                key={tab}
                type="button"
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
                {!metricsLoading &&
                  metrics != null &&
                  activeTab === 'system' && (
                    <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
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
                      </div>
                      <MetricCard
                        label="Load Avg"
                        value={`${metrics.loadAvg1.toFixed(2)}`}
                        sub={`${metrics.loadAvg5.toFixed(2)} / ${metrics.loadAvg15.toFixed(2)}`}
                      />
                    </div>
                  )}
                {!metricsLoading &&
                  metrics != null &&
                  activeTab === 'runtime' && (
                    <div className="grid grid-cols-2 gap-2">
                      <MetricCard
                        label="Goroutines"
                        value={`${metrics.numGoroutines}`}
                      />
                      <MetricCard
                        label="Go Heap"
                        value={`${metrics.goMemAllocMB.toFixed(1)} MB`}
                      />
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
          <span className="min-w-0 flex-1 truncate">
            {overviewError !== ''
              ? overviewError
              : overviewLoading
                ? 'Loading metrics...'
                : 'Metrics connected'}
          </span>
          <span className="shrink-0 whitespace-nowrap">
            {metrics?.collectedAt
              ? `updated ${formatTimeAgo(metrics.collectedAt)}`
              : 'waiting'}
          </span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/metrics')({
  component: MetricsPage,
})
