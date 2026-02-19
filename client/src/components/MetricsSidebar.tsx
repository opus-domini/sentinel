import { useMemo, useState } from 'react'
import { Lock, LockOpen } from 'lucide-react'
import type { OpsOverview } from '@/types'
import MetricsHelpDialog from '@/components/MetricsHelpDialog'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { formatUptime } from '@/lib/opsUtils'

type MetricsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  overview: OpsOverview | null
  onTokenChange: (value: string) => void
}

export default function MetricsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  authenticated,
  overview,
  onTokenChange,
}: MetricsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

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
                  {authenticated ? (
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
            authenticated={authenticated}
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
          </div>
        </section>
      </div>
    </SidebarShell>
  )
}
