import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  Clock3,
  Cpu,
  Database,
  Gauge,
  HardDrive,
  Layers3,
  MemoryStick,
  Network,
  ServerCog,
  ShieldAlert,
  Waves,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { OpsHostMetrics, OpsMetricsResponse, OpsOverviewResponse } from '@/types'
import type { MetricsSnapshot } from '@/lib/MetricsHistory'
import type { MetricSeverity } from '@/lib/metricsView'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useMetaContext } from '@/contexts/MetaContext'
import { useOpsEvents, useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { MetricsHistory } from '@/lib/MetricsHistory'
import { OPS_METRICS_QUERY_KEY, OPS_OVERVIEW_QUERY_KEY, isOpsWsMessage } from '@/lib/opsQueryCache'
import { formatBytes, toErrorMessage } from '@/lib/opsUtils'
import { ProgressBar } from '@/lib/ProgressBar'
import { Sparkline } from '@/lib/Sparkline'
import {
  computeByteRate,
  formatByteRate,
  formatDurationLong,
  formatPercentValue,
  percentSeverity,
  pressureSeverity,
} from '@/lib/metricsView'
import { cn } from '@/lib/utils'

const SPARKLINE_COLORS = {
  cpu: 'var(--chart-cpu)',
  memory: 'var(--chart-memory)',
  disk: 'var(--chart-disk)',
  loadAvg: 'var(--chart-load)',
  swap: 'var(--chart-swap)',
  inodes: 'var(--chart-inodes)',
  networkRx: 'var(--chart-network-rx)',
  networkTx: 'var(--chart-network-tx)',
  pressure: 'var(--chart-pressure)',
  process: 'var(--chart-process)',
  goroutines: 'var(--chart-goroutines)',
  goHeap: 'var(--chart-go-heap)',
} as const

const METRICS_HERO_SKELETON_KEYS = [
  'metrics-hero-cpu',
  'metrics-hero-memory',
  'metrics-hero-disk',
  'metrics-hero-load',
] as const
const METRICS_MINI_SKELETON_KEYS = [
  'metrics-mini-swap',
  'metrics-mini-inodes',
  'metrics-mini-processes',
  'metrics-mini-threads',
  'metrics-mini-pressure',
  'metrics-mini-network',
  'metrics-mini-runtime',
  'metrics-mini-gc',
] as const

type MetricsTab = 'saturation' | 'network' | 'runtime'

const METRICS_TABS: Array<{
  id: MetricsTab
  label: string
  detail: string
  Icon: LucideIcon
}> = [
  {
    id: 'saturation',
    label: 'Saturation',
    detail: 'capacity, pressure, process load',
    Icon: Gauge,
  },
  {
    id: 'network',
    label: 'Network',
    detail: 'interfaces, ingress and egress',
    Icon: Network,
  },
  {
    id: 'runtime',
    label: 'Runtime',
    detail: 'Sentinel process health',
    Icon: ServerCog,
  },
]

const numberFormatter = new Intl.NumberFormat('en-US')

const round1 = (n: number) => Math.round(n * 10) / 10
const round2 = (n: number) => Math.round(n * 100) / 100

const SAMPLE_RATE = {
  live: '~2s',
  disk: '~10s',
  process: '~10s',
  pressure: '~10s',
  uptime: '~30s',
} as const

const METRIC_ICON_COLOR = 'var(--brand-glow)'

function metricTooltip(title: string, sampleRate?: string): string {
  if (sampleRate == null || sampleRate.trim() === '') {
    return title
  }
  return `${title}\nSample rate: ${sampleRate}`
}

function toSnapshot(m: OpsHostMetrics): MetricsSnapshot {
  return {
    cpuPercent: round1(m.cpuPercent),
    memPercent: round1(m.memPercent),
    diskPercent: round1(m.diskPercent),
    diskInodesPercent: round1(m.diskInodesPercent),
    swapPercent: round1(m.swapPercent),
    loadAvg1: round2(m.loadAvg1),
    loadPerCPU: round2(m.loadPerCPU),
    netRxBytes: m.netRxBytes,
    netTxBytes: m.netTxBytes,
    processCount: m.processCount,
    threadCount: m.threadCount,
    cpuPressureAvg10: round2(m.cpuPressureAvg10),
    memPressureAvg10: round2(m.memPressureAvg10),
    ioPressureAvg10: round2(m.ioPressureAvg10),
    numGoroutines: m.numGoroutines,
    goMemAllocMB: round1(m.goMemAllocMB),
    goMemSysMB: round1(m.goMemSysMB),
    goLastGcPauseMs: round2(m.goLastGcPauseMs),
  }
}

function formatBootTime(value: string): string {
  const parsed = Date.parse(value)
  if (Number.isNaN(parsed)) return '-'
  return new Date(parsed).toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

function formatCount(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '-'
  return numberFormatter.format(Math.trunc(value))
}

function formatMaybeBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '-'
  return formatBytes(value)
}

function severityRank(severity: MetricSeverity): number {
  if (severity === 'critical') return 3
  if (severity === 'warn') return 2
  if (severity === 'unknown') return 1
  return 0
}

function buildRisk(metrics: OpsHostMetrics | null): {
  severity: MetricSeverity
  label: string
  detail: string
} {
  if (metrics == null) {
    return { severity: 'unknown', label: 'Waiting', detail: 'no sample' }
  }

  const signals: Array<MetricSeverity> = [
    percentSeverity(metrics.cpuPercent, 80, 90),
    percentSeverity(metrics.memPercent, 80, 90),
    percentSeverity(metrics.diskPercent, 85, 95),
    percentSeverity(metrics.diskInodesPercent, 80, 90),
    metrics.swapTotalBytes > 0 ? percentSeverity(metrics.swapPercent, 20, 60) : 'ok',
    pressureSeverity(metrics.cpuPressureAvg10),
    pressureSeverity(metrics.memPressureAvg10),
    pressureSeverity(metrics.ioPressureAvg10),
  ]
  const critical = signals.filter((signal) => signal === 'critical').length
  const warn = signals.filter((signal) => signal === 'warn').length
  const severity = signals.reduce<MetricSeverity>(
    (worst, signal) => (severityRank(signal) > severityRank(worst) ? signal : worst),
    'ok',
  )

  if (critical > 0) {
    return {
      severity,
      label: 'Critical',
      detail: `${critical} hard signal${critical === 1 ? '' : 's'}`,
    }
  }
  if (warn > 0) {
    return {
      severity,
      label: 'Attention',
      detail: `${warn} warm signal${warn === 1 ? '' : 's'}`,
    }
  }
  return { severity, label: 'Nominal', detail: 'all key signals green' }
}

function trendFor(
  snapshots: Array<MetricsSnapshot>,
  timestamps: Array<number>,
  key: keyof MetricsSnapshot,
  { min = Number.NEGATIVE_INFINITY }: { min?: number } = {},
): { values: Array<number>; timestamps: Array<number> } {
  const values: Array<number> = []
  const filteredTimestamps: Array<number> = []
  snapshots.forEach((snapshot, index) => {
    const value = snapshot[key]
    if (!Number.isFinite(value) || value < min) return
    values.push(value)
    filteredTimestamps.push(timestamps[index])
  })
  return { values, timestamps: filteredTimestamps }
}

type MetricsFooterSummaryParams = {
  overviewError: string
  metricsError: string
  overviewLoading: boolean
  metricsLoading: boolean
}

function buildMetricsFooterSummary({
  overviewError,
  metricsError,
  overviewLoading,
  metricsLoading,
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
  return 'Metrics ready'
}

function severityClass(severity: MetricSeverity): string {
  switch (severity) {
    case 'critical':
      return 'border-destructive/45 bg-destructive/10 text-destructive-foreground'
    case 'warn':
      return 'border-warning/45 bg-warning/10 text-warning-foreground'
    case 'unknown':
      return 'border-border-subtle bg-surface-elevated text-muted-foreground'
    default:
      return 'border-brand-glow/25 bg-[color-mix(in_srgb,var(--brand-glow)_7%,var(--surface-elevated))] text-primary-text'
  }
}

function StatusPill({ severity, label }: { severity: MetricSeverity; label: string }) {
  return (
    <span
      className={cn(
        'inline-flex h-5 items-center rounded border px-1.5 text-[10px] font-medium',
        severityClass(severity),
      )}
    >
      {label}
    </span>
  )
}

function SectionHeading({ title, detail }: { title: string; detail?: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-2">
      <h2 className="truncate text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
        {title}
      </h2>
      {detail && <span className="shrink-0 text-[10px] text-muted-foreground">{detail}</span>}
    </div>
  )
}

function MetricTitle({
  title,
  Icon,
  sampleRate,
  iconClassName = 'h-7 w-7',
}: {
  title: string
  Icon: LucideIcon
  sampleRate?: string
  iconClassName?: string
}) {
  return (
    <TooltipHelper content={metricTooltip(title, sampleRate)}>
      <span
        aria-label={metricTooltip(title, sampleRate).replace('\n', '. ')}
        className="flex min-w-0 cursor-help items-center gap-2 rounded-sm"
      >
        <span
          className={cn(
            'flex shrink-0 items-center justify-center rounded-md border border-border-subtle bg-surface-overlay',
            iconClassName,
          )}
          style={{ color: METRIC_ICON_COLOR }}
        >
          <Icon className="h-3.5 w-3.5" />
        </span>
        <span className="truncate text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
          {title}
        </span>
      </span>
    </TooltipHelper>
  )
}

function MetricPanel({
  title,
  value,
  detail,
  Icon,
  severity = 'ok',
  percent,
  trend,
  trendColor,
  trendDomain,
  formatTrendValue,
  className,
  chartClassName,
  sampleRate,
}: {
  title: string
  value: string
  detail?: string
  Icon: LucideIcon
  severity?: MetricSeverity
  percent?: number
  trend?: { values: Array<number>; timestamps: Array<number> }
  trendColor: string
  trendDomain?: [number, number]
  formatTrendValue?: (value: number) => string
  className?: string
  chartClassName?: string
  sampleRate?: string
}) {
  const showProgress = typeof percent === 'number' && Number.isFinite(percent)

  return (
    <div
      className={cn(
        'grid min-h-[156px] grid-rows-[auto_auto_1fr] overflow-hidden rounded-lg border p-3',
        severity === 'ok' ? 'border-border-subtle bg-surface-elevated' : severityClass(severity),
        className,
      )}
    >
      <div className="flex min-w-0 items-center justify-between gap-2">
        <MetricTitle title={title} Icon={Icon} sampleRate={sampleRate} />
        <StatusPill severity={severity} label={severity} />
      </div>
      <div className="mt-3 min-w-0">
        <p className="truncate text-[24px] font-semibold leading-none text-foreground">{value}</p>
        {detail && <p className="mt-1 text-[10px] leading-4 text-muted-foreground">{detail}</p>}
        {showProgress && <ProgressBar percent={Math.max(0, percent)} />}
      </div>
      <div className={cn('mt-3 h-12 min-h-0 w-full', chartClassName)}>
        {trend && trend.values.length >= 2 ? (
          <Sparkline
            data={trend.values}
            timestamps={trend.timestamps}
            color={trendColor}
            domain={trendDomain}
            formatValue={
              formatTrendValue ?? ((v) => (trendDomain ? formatPercentValue(v) : `${v}`))
            }
            className="h-full w-full"
          />
        ) : (
          <div className="h-full rounded border border-dashed border-border-subtle bg-surface-overlay/50" />
        )}
      </div>
    </div>
  )
}

function MiniStat({
  label,
  value,
  sub,
  Icon,
  severity = 'ok',
  className,
  sampleRate,
}: {
  label: string
  value: string
  sub?: string
  Icon: LucideIcon
  severity?: MetricSeverity
  className?: string
  sampleRate?: string
}) {
  return (
    <div
      className={cn(
        'grid min-h-[96px] grid-cols-[28px_1fr] gap-2 rounded-lg border p-2.5',
        severity === 'ok' ? 'border-border-subtle bg-surface-elevated' : severityClass(severity),
        className,
      )}
    >
      <div className="contents">
        <div className="col-span-2">
          <MetricTitle title={label} Icon={Icon} sampleRate={sampleRate} />
        </div>
      </div>
      <div className="col-span-2 min-w-0 pl-[36px]">
        <p className="mt-1 truncate text-[14px] font-semibold text-foreground">{value}</p>
        {sub && <p className="text-[10px] leading-3 text-muted-foreground">{sub}</p>}
      </div>
    </div>
  )
}

function PostureStat({
  label,
  value,
  sub,
  Icon,
  sampleRate,
}: {
  label: string
  value: string
  sub?: string
  Icon: LucideIcon
  sampleRate?: string
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 border-t border-border-subtle pt-3 lg:border-l lg:border-t-0 lg:pl-4 lg:pt-0">
      <div className="min-w-0">
        <MetricTitle title={label} Icon={Icon} sampleRate={sampleRate} iconClassName="h-8 w-8" />
        <p className="mt-0.5 text-[18px] font-semibold leading-none text-foreground">{value}</p>
        {sub && <p className="mt-1 text-[10px] leading-3 text-secondary-foreground">{sub}</p>}
      </div>
    </div>
  )
}

function MetricsTabButton({
  tab,
  active,
  onSelect,
}: {
  tab: (typeof METRICS_TABS)[number]
  active: boolean
  onSelect: (tab: MetricsTab) => void
}) {
  return (
    <button
      id={`metrics-tab-${tab.id}`}
      type="button"
      role="tab"
      aria-label={`${tab.label}: ${tab.detail}`}
      aria-selected={active}
      aria-controls={`metrics-panel-${tab.id}`}
      className={cn(
        'grid min-w-0 grid-cols-[28px_1fr] items-center gap-2 rounded-md border px-2.5 py-2 text-left transition-colors',
        active
          ? 'border-brand-glow/40 bg-[color-mix(in_srgb,var(--brand-glow)_12%,var(--surface-elevated))] text-foreground'
          : 'border-transparent bg-transparent text-muted-foreground hover:border-border-subtle hover:bg-surface-hover hover:text-foreground',
      )}
      onClick={() => onSelect(tab.id)}
    >
      <span
        className={cn(
          'flex h-7 w-7 items-center justify-center rounded-md border',
          active
            ? 'border-brand-glow/30 bg-brand-ink/60 text-brand-glow'
            : 'border-border-subtle bg-surface-overlay',
        )}
      >
        <tab.Icon className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0">
        <span className="block text-[11px] font-semibold uppercase tracking-[0.08em]">
          {tab.label}
        </span>
        <span className="block text-[10px] leading-3 text-muted-foreground">{tab.detail}</span>
      </span>
    </button>
  )
}

function MetricsSkeleton() {
  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-4">
        {METRICS_HERO_SKELETON_KEYS.map((key) => (
          <div
            key={key}
            className="h-[146px] rounded-lg border border-border-subtle bg-surface-elevated motion-safe:animate-pulse"
          />
        ))}
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {METRICS_MINI_SKELETON_KEYS.map((key) => (
          <div
            key={key}
            className="h-[86px] rounded-lg border border-border-subtle bg-surface-elevated motion-safe:animate-pulse"
          />
        ))}
      </div>
    </div>
  )
}

function MetricsPage() {
  const { hostname } = useMetaContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const historyRef = useRef(new MetricsHistory())
  const seededRef = useRef(false)
  const [activeTab, setActiveTab] = useState<MetricsTab>('saturation')

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
    metricsQuery.error != null ? toErrorMessage(metricsQuery.error, 'failed to load metrics') : ''

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
  const forceReconnectOpsEvents = useOpsEventsReconnect()
  const resyncPage = useCallback(() => {
    forceReconnectOpsEvents()
    refreshPage()
  }, [forceReconnectOpsEvents, refreshPage])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      if (!isOpsWsMessage(message)) return
      switch (message.type) {
        case 'ops.overview.updated':
          queryClient.setQueryData(OPS_OVERVIEW_QUERY_KEY, message.payload.overview)
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

  const connectionState = useOpsEvents(handleWSMessage)
  const footerSummary = buildMetricsFooterSummary({
    overviewError,
    metricsError,
    overviewLoading,
    metricsLoading,
  })

  const history = historyRef.current
  const snapshots = history.toArray()
  const ts = history.timestamps()
  const trends = useMemo(
    () => ({
      cpu: trendFor(snapshots, ts, 'cpuPercent', { min: 0 }),
      memory: trendFor(snapshots, ts, 'memPercent', { min: 0 }),
      disk: trendFor(snapshots, ts, 'diskPercent', { min: 0 }),
      load: trendFor(snapshots, ts, 'loadAvg1', { min: 0 }),
      swap: trendFor(snapshots, ts, 'swapPercent', { min: 0 }),
      inodes: trendFor(snapshots, ts, 'diskInodesPercent', { min: 0 }),
      rx: trendFor(snapshots, ts, 'netRxBytes', { min: 0 }),
      tx: trendFor(snapshots, ts, 'netTxBytes', { min: 0 }),
      processes: trendFor(snapshots, ts, 'processCount', { min: 0 }),
      threads: trendFor(snapshots, ts, 'threadCount', { min: 0 }),
      cpuPressure: trendFor(snapshots, ts, 'cpuPressureAvg10', { min: 0 }),
      memPressure: trendFor(snapshots, ts, 'memPressureAvg10', { min: 0 }),
      ioPressure: trendFor(snapshots, ts, 'ioPressureAvg10', { min: 0 }),
      goroutines: trendFor(snapshots, ts, 'numGoroutines', { min: 0 }),
      heap: trendFor(snapshots, ts, 'goMemAllocMB', { min: 0 }),
      goSys: trendFor(snapshots, ts, 'goMemSysMB', { min: 0 }),
      gcPause: trendFor(snapshots, ts, 'goLastGcPauseMs', { min: 0 }),
    }),
    [snapshots, ts],
  )

  const rxRate = computeByteRate(trends.rx.values, trends.rx.timestamps)
  const txRate = computeByteRate(trends.tx.values, trends.tx.timestamps)
  const risk = buildRisk(metrics)
  const activeTabMeta = METRICS_TABS.find((tab) => tab.id === activeTab) ?? METRICS_TABS[0]

  return (
    <AppShell>
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_18%_-12%,var(--section-glow-brand),transparent_34%),radial-gradient(circle_at_88%_0%,var(--section-glow-ok),transparent_28%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <AppSectionTitle hostname={hostname} section="metrics" />
          </div>
          <div className="flex items-center gap-1.5">
            <StatusPill severity={risk.severity} label={risk.label} />
            <ConnectionBadge state={connectionState} onClick={resyncPage} />
          </div>
        </header>

        <ScrollArea className="h-full min-h-0 overflow-hidden">
          <div className="grid gap-4 p-3">
            {metricsLoading && <MetricsSkeleton />}

            {metricsError !== '' && (
              <div className="grid gap-2 rounded-lg border border-dashed border-destructive/40 bg-destructive/10 p-3">
                <p className="text-[12px] text-destructive-foreground">{metricsError}</p>
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
              <EmptyState variant="inline" className="grid gap-2 p-3 text-[12px]">
                <p>No metric sample received yet.</p>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 w-fit text-[11px]"
                  onClick={refreshPage}
                >
                  Refresh metrics
                </Button>
              </EmptyState>
            )}

            {!metricsLoading && metrics != null && (
              <>
                <section className="grid gap-3">
                  <div className="grid overflow-hidden rounded-lg border border-brand-glow/30 bg-[linear-gradient(135deg,color-mix(in_srgb,var(--brand-glow)_13%,var(--surface-elevated)),var(--surface-elevated)_56%,color-mix(in_srgb,var(--brand-accent)_10%,var(--surface-overlay)))] p-3 sm:p-4">
                    <div className="grid gap-4 lg:grid-cols-[1.45fr_1fr_1fr_1fr] lg:items-center">
                      <div className="flex min-w-0 items-center justify-between gap-3">
                        <div className="min-w-0 space-y-1">
                          <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-primary-text">
                            Host posture
                          </p>
                          <p className="text-[28px] font-semibold leading-none text-foreground">
                            {risk.label}
                          </p>
                          <p className="text-[11px] leading-4 text-secondary-foreground">
                            {risk.detail}
                          </p>
                        </div>
                        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-brand-glow/25 bg-brand-ink/60 text-brand-glow">
                          <ShieldAlert className="h-4 w-4" />
                        </span>
                      </div>
                      <PostureStat
                        label="Host up"
                        value={formatDurationLong(metrics.hostUptimeSec)}
                        sub={`boot ${formatBootTime(metrics.bootTime)}`}
                        Icon={Clock3}
                        sampleRate={SAMPLE_RATE.uptime}
                      />
                      <PostureStat
                        label="Threads"
                        value={formatCount(metrics.threadCount)}
                        sub={`${formatCount(metrics.processCount)} processes`}
                        Icon={Layers3}
                        sampleRate={SAMPLE_RATE.process}
                      />
                      <PostureStat
                        label="Load / core"
                        value={metrics.loadPerCPU >= 0 ? metrics.loadPerCPU.toFixed(2) : '-'}
                        sub={`${metrics.loadAvg1.toFixed(2)} load · ${metrics.cpuCount} cores`}
                        Icon={Gauge}
                        sampleRate={SAMPLE_RATE.live}
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
                    <MetricPanel
                      title="CPU"
                      value={formatPercentValue(metrics.cpuPercent)}
                      detail={`${metrics.cpuCount} cores · load ${metrics.loadAvg1.toFixed(2)}`}
                      Icon={Cpu}
                      severity={percentSeverity(metrics.cpuPercent, 80, 90)}
                      percent={metrics.cpuPercent}
                      trend={trends.cpu}
                      trendColor={SPARKLINE_COLORS.cpu}
                      trendDomain={[0, 100]}
                      sampleRate={SAMPLE_RATE.live}
                    />
                    <MetricPanel
                      title="Memory"
                      value={formatPercentValue(metrics.memPercent)}
                      detail={`${formatBytes(metrics.memUsedBytes)} used · ${formatMaybeBytes(metrics.memAvailableBytes)} free`}
                      Icon={MemoryStick}
                      severity={percentSeverity(metrics.memPercent, 80, 90)}
                      percent={metrics.memPercent}
                      trend={trends.memory}
                      trendColor={SPARKLINE_COLORS.memory}
                      trendDomain={[0, 100]}
                      sampleRate={SAMPLE_RATE.live}
                    />
                    <MetricPanel
                      title="Root disk"
                      value={formatPercentValue(metrics.diskPercent)}
                      detail={`${formatMaybeBytes(metrics.diskFreeBytes)} free · ${formatBytes(metrics.diskTotalBytes)} total`}
                      Icon={HardDrive}
                      severity={percentSeverity(metrics.diskPercent, 85, 95)}
                      percent={metrics.diskPercent}
                      trend={trends.disk}
                      trendColor={SPARKLINE_COLORS.disk}
                      trendDomain={[0, 100]}
                      sampleRate={SAMPLE_RATE.disk}
                    />
                  </div>
                </section>

                <section className="grid gap-2">
                  <div
                    role="tablist"
                    aria-label="Metric contexts"
                    className="grid grid-cols-1 gap-1 rounded-lg border border-border-subtle bg-surface-overlay p-1 sm:grid-cols-3"
                  >
                    {METRICS_TABS.map((tab) => (
                      <MetricsTabButton
                        key={tab.id}
                        tab={tab}
                        active={activeTab === tab.id}
                        onSelect={setActiveTab}
                      />
                    ))}
                  </div>
                </section>

                <section
                  id={`metrics-panel-${activeTab}`}
                  role="tabpanel"
                  aria-labelledby={`metrics-tab-${activeTab}`}
                  className="grid gap-2"
                >
                  <SectionHeading title={activeTabMeta.label} detail={activeTabMeta.detail} />

                  {activeTab === 'saturation' && (
                    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2 2xl:grid-cols-3">
                      <MetricPanel
                        title="Load / core"
                        value={metrics.loadPerCPU >= 0 ? metrics.loadPerCPU.toFixed(2) : '-'}
                        detail={`1m ${metrics.loadAvg1.toFixed(2)} · 5m ${metrics.loadAvg5.toFixed(2)} · 15m ${metrics.loadAvg15.toFixed(2)} · ${metrics.cpuCount} cores`}
                        Icon={Gauge}
                        severity={percentSeverity(metrics.loadPerCPU * 100, 70, 100)}
                        percent={metrics.loadPerCPU >= 0 ? metrics.loadPerCPU * 100 : undefined}
                        trend={trends.load}
                        trendColor={SPARKLINE_COLORS.loadAvg}
                        formatTrendValue={(value) => value.toFixed(2)}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.live}
                      />
                      <MetricPanel
                        title="Swap"
                        value={
                          metrics.swapTotalBytes > 0
                            ? formatPercentValue(metrics.swapPercent)
                            : 'none'
                        }
                        detail={
                          metrics.swapTotalBytes > 0
                            ? `${formatBytes(metrics.swapUsedBytes)} used · ${formatBytes(metrics.swapTotalBytes)} total`
                            : 'swap is not configured on this host'
                        }
                        Icon={Database}
                        severity={
                          metrics.swapTotalBytes > 0
                            ? percentSeverity(metrics.swapPercent, 20, 60)
                            : 'ok'
                        }
                        percent={metrics.swapTotalBytes > 0 ? metrics.swapPercent : undefined}
                        trend={metrics.swapTotalBytes > 0 ? trends.swap : undefined}
                        trendColor={SPARKLINE_COLORS.swap}
                        trendDomain={[0, 100]}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.live}
                      />
                      <MetricPanel
                        title="Disk inodes"
                        value={
                          metrics.diskInodesTotal > 0
                            ? formatPercentValue(metrics.diskInodesPercent)
                            : '-'
                        }
                        detail={
                          metrics.diskInodesTotal > 0
                            ? `${formatCount(metrics.diskInodesUsed)} used · ${formatCount(metrics.diskInodesTotal)} total`
                            : 'inode usage is not reported by this filesystem'
                        }
                        Icon={Database}
                        severity={percentSeverity(metrics.diskInodesPercent, 80, 90)}
                        percent={
                          metrics.diskInodesTotal > 0 ? metrics.diskInodesPercent : undefined
                        }
                        trend={metrics.diskInodesTotal > 0 ? trends.inodes : undefined}
                        trendColor={SPARKLINE_COLORS.inodes}
                        trendDomain={[0, 100]}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.disk}
                      />
                      <MetricPanel
                        title="CPU pressure"
                        value={
                          metrics.cpuPressureAvg10 >= 0 ? metrics.cpuPressureAvg10.toFixed(2) : '-'
                        }
                        detail="Linux PSI avg10 for CPU stalls"
                        Icon={Waves}
                        severity={pressureSeverity(metrics.cpuPressureAvg10)}
                        trend={trends.cpuPressure}
                        trendColor={SPARKLINE_COLORS.pressure}
                        formatTrendValue={(value) => value.toFixed(2)}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.pressure}
                      />
                      <MetricPanel
                        title="Memory pressure"
                        value={
                          metrics.memPressureAvg10 >= 0 ? metrics.memPressureAvg10.toFixed(2) : '-'
                        }
                        detail="Linux PSI avg10 for memory stalls"
                        Icon={Waves}
                        severity={pressureSeverity(metrics.memPressureAvg10)}
                        trend={trends.memPressure}
                        trendColor={SPARKLINE_COLORS.pressure}
                        formatTrendValue={(value) => value.toFixed(2)}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.pressure}
                      />
                      <MetricPanel
                        title="IO pressure"
                        value={
                          metrics.ioPressureAvg10 >= 0 ? metrics.ioPressureAvg10.toFixed(2) : '-'
                        }
                        detail="Linux PSI avg10 for I/O stalls"
                        Icon={Waves}
                        severity={pressureSeverity(metrics.ioPressureAvg10)}
                        trend={trends.ioPressure}
                        trendColor={SPARKLINE_COLORS.pressure}
                        formatTrendValue={(value) => value.toFixed(2)}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.pressure}
                      />
                      <MetricPanel
                        title="Processes"
                        value={formatCount(metrics.processCount)}
                        detail={`${formatCount(metrics.threadCount)} threads on this host`}
                        Icon={Layers3}
                        trend={trends.processes}
                        trendColor={SPARKLINE_COLORS.process}
                        formatTrendValue={formatCount}
                        className="min-h-[190px]"
                        chartClassName="h-20"
                        sampleRate={SAMPLE_RATE.process}
                      />
                    </div>
                  )}

                  {activeTab === 'network' && (
                    <div className="grid gap-3">
                      <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
                        <MetricPanel
                          title="Ingress"
                          value={formatByteRate(rxRate)}
                          detail={`${formatMaybeBytes(metrics.netRxBytes)} received across non-loopback interfaces`}
                          Icon={ArrowDownToLine}
                          trend={trends.rx}
                          trendColor={SPARKLINE_COLORS.networkRx}
                          formatTrendValue={formatBytes}
                          className="min-h-[230px]"
                          chartClassName="h-28"
                          sampleRate={SAMPLE_RATE.live}
                        />
                        <MetricPanel
                          title="Egress"
                          value={formatByteRate(txRate)}
                          detail={`${formatMaybeBytes(metrics.netTxBytes)} sent across non-loopback interfaces`}
                          Icon={ArrowUpFromLine}
                          trend={trends.tx}
                          trendColor={SPARKLINE_COLORS.networkTx}
                          formatTrendValue={formatBytes}
                          className="min-h-[230px]"
                          chartClassName="h-28"
                          sampleRate={SAMPLE_RATE.live}
                        />
                      </div>
                      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                        <MiniStat
                          label="Interfaces"
                          value={formatCount(metrics.netInterfaces)}
                          sub="non-loopback devices included in RX/TX totals"
                          Icon={Network}
                          sampleRate={SAMPLE_RATE.live}
                        />
                        <MiniStat
                          label="RX total"
                          value={formatMaybeBytes(metrics.netRxBytes)}
                          sub="cumulative bytes received since boot"
                          Icon={ArrowDownToLine}
                          sampleRate={SAMPLE_RATE.live}
                        />
                        <MiniStat
                          label="TX total"
                          value={formatMaybeBytes(metrics.netTxBytes)}
                          sub="cumulative bytes sent since boot"
                          Icon={ArrowUpFromLine}
                          sampleRate={SAMPLE_RATE.live}
                        />
                      </div>
                    </div>
                  )}

                  {activeTab === 'runtime' && (
                    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                      <MetricPanel
                        title="Goroutines"
                        value={formatCount(metrics.numGoroutines)}
                        detail={`Sentinel PID ${overview?.sentinel.pid ?? '-'}`}
                        Icon={Activity}
                        trend={trends.goroutines}
                        trendColor={SPARKLINE_COLORS.goroutines}
                        formatTrendValue={formatCount}
                        className="min-h-[210px]"
                        chartClassName="h-24"
                        sampleRate={SAMPLE_RATE.live}
                      />
                      <MetricPanel
                        title="Heap alloc"
                        value={`${metrics.goMemAllocMB.toFixed(1)} MB`}
                        detail={`${metrics.goMemSysMB.toFixed(1)} MB reserved from the Go runtime`}
                        Icon={ServerCog}
                        trend={trends.heap}
                        trendColor={SPARKLINE_COLORS.goHeap}
                        formatTrendValue={(value) => `${value.toFixed(1)} MB`}
                        className="min-h-[210px]"
                        chartClassName="h-24"
                        sampleRate={SAMPLE_RATE.live}
                      />
                      <MetricPanel
                        title="GC"
                        value={formatCount(metrics.goNumGC)}
                        detail={`${metrics.goLastGcPauseMs.toFixed(2)} ms latest pause`}
                        Icon={Gauge}
                        trend={trends.gcPause}
                        trendColor={SPARKLINE_COLORS.pressure}
                        formatTrendValue={(value) => `${value.toFixed(2)} ms`}
                        className="min-h-[210px]"
                        chartClassName="h-24"
                        sampleRate={SAMPLE_RATE.live}
                      />
                      <MetricPanel
                        title="Host load"
                        value={metrics.loadAvg1.toFixed(2)}
                        detail={`${metrics.loadAvg5.toFixed(2)} / ${metrics.loadAvg15.toFixed(2)} load averages · ${metrics.cpuCount} cores`}
                        Icon={Gauge}
                        severity={percentSeverity(metrics.loadPerCPU * 100, 70, 100)}
                        trend={trends.load}
                        trendColor={SPARKLINE_COLORS.loadAvg}
                        formatTrendValue={(value) => value.toFixed(2)}
                        className="min-h-[210px]"
                        chartClassName="h-24"
                        sampleRate={SAMPLE_RATE.live}
                      />
                    </div>
                  )}
                </section>
              </>
            )}
          </div>
        </ScrollArea>

        <footer
          aria-live="polite"
          className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground"
        >
          <span className="min-w-0 flex-1 truncate">{footerSummary}</span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/metrics')({
  component: MetricsPage,
})
