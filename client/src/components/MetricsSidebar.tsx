import { useMemo, useState } from 'react'
import { Lock, LockOpen } from 'lucide-react'
import type { OpsHostMetrics, OpsOverview } from '@/types'
import MetricsHelpDialog from '@/components/MetricsHelpDialog'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { formatUptime } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

type MetricsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  token: string
  overview: OpsOverview | null
  metrics: OpsHostMetrics | null
  autoRefresh: boolean
  onAutoRefreshChange: (value: boolean) => void
  onTokenChange: (value: string) => void
}

export default function MetricsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  token,
  overview,
  metrics,
  autoRefresh,
  onAutoRefreshChange,
  onTokenChange,
}: MetricsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const hasToken = token.trim() !== ''

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return hasToken ? 'Token configured (required)' : 'Token required'
    }
    return hasToken ? 'Token configured' : 'No token'
  }, [hasToken, tokenRequired])

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Metrics
            </span>
            <div className="ml-auto flex items-center gap-1.5">
              <MetricsHelpDialog />
              <TooltipHelper content={lockLabel}>
                <Button
                  variant="ghost"
                  size="icon"
                  className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => setIsTokenOpen(true)}
                  aria-label="API token"
                >
                  {hasToken ? (
                    <Lock className="h-4 w-4" />
                  ) : (
                    <LockOpen className="h-4 w-4" />
                  )}
                </Button>
              </TooltipHelper>
            </div>
          </div>

          <TokenDialog
            open={isTokenOpen}
            onOpenChange={setIsTokenOpen}
            token={token}
            onTokenChange={onTokenChange}
            tokenRequired={tokenRequired}
          />
        </section>

        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Host
          </span>
          <div className="grid gap-1.5">
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Hostname</p>
              <p className="mt-0.5 min-w-0 truncate text-[11px] font-medium">
                {overview?.host.hostname || '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">OS / Arch</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview != null
                  ? `${overview.host.os} / ${overview.host.arch}`
                  : '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">CPUs</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview?.host.cpus ?? '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Go</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview?.host.goVersion || '-'}
              </p>
            </div>
          </div>
        </section>

        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Sentinel
          </span>
          <div className="grid gap-1.5">
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">PID</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview?.sentinel.pid ?? '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Uptime</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview != null
                  ? formatUptime(overview.sentinel.uptimeSec)
                  : '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">
                Last collected
              </p>
              <p className="mt-0.5 min-w-0 truncate text-[11px] font-medium">
                {metrics?.collectedAt || '-'}
              </p>
            </div>
          </div>
        </section>

        <section className="rounded-lg border border-border-subtle bg-secondary p-2">
          <label className="flex cursor-pointer items-center justify-between gap-2 select-none">
            <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Auto-refresh
            </span>
            <div className="flex items-center gap-1.5">
              <button
                type="button"
                role="switch"
                aria-checked={autoRefresh}
                onClick={() => onAutoRefreshChange(!autoRefresh)}
                className={cn(
                  'relative inline-flex h-4 w-7 shrink-0 cursor-pointer items-center rounded-full border transition-colors',
                  autoRefresh
                    ? 'border-emerald-500/60 bg-emerald-500/30'
                    : 'border-border bg-surface-elevated',
                )}
              >
                <span
                  className={cn(
                    'pointer-events-none block h-3 w-3 rounded-full bg-foreground shadow transition-transform',
                    autoRefresh ? 'translate-x-3' : 'translate-x-0',
                  )}
                />
              </button>
              <span className="text-[10px] text-muted-foreground">5s</span>
            </div>
          </label>
        </section>
      </div>
    </SidebarShell>
  )
}
