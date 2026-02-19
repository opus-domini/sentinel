import { useMemo, useState } from 'react'
import { Lock, LockOpen } from 'lucide-react'
import type { OpsOverview } from '@/types'
import AlertsHelpDialog from '@/components/AlertsHelpDialog'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { formatUptime } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

type AlertsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  alertCount: number
  openCount: number
  overview: OpsOverview | null
  selectedSeverity: string
  onSeverityChange: (value: string) => void
  onTokenChange: (value: string) => void
}

const severities = ['all', 'warn', 'error'] as const

export default function AlertsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  authenticated,
  alertCount,
  openCount,
  overview,
  selectedSeverity,
  onSeverityChange,
  onTokenChange,
}: AlertsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

  const health = useMemo(() => {
    if (overview == null) return '-'
    return overview.services.failed > 0
      ? `${overview.services.failed} failed`
      : 'healthy'
  }, [overview])

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Alerts
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {alertCount}
            </span>
            {openCount > 0 && (
              <span className="rounded-full bg-amber-500/20 px-1.5 text-[10px] text-amber-200">
                {openCount} open
              </span>
            )}
            <div className="ml-auto flex items-center gap-1.5">
              <AlertsHelpDialog />
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
            Overview
          </span>
          <div className="grid gap-1.5">
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Host</p>
              <p className="mt-0.5 min-w-0 truncate text-[11px] font-medium">
                {overview != null
                  ? `${overview.host.hostname || '-'} (${overview.host.os}/${overview.host.arch})`
                  : '-'}
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
              <p className="text-[10px] text-muted-foreground">Health</p>
              <p className="mt-0.5 text-[11px] font-medium">{health}</p>
            </div>
          </div>
        </section>

        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Severity
          </span>
          <div className="flex gap-1">
            {severities.map((sev) => (
              <button
                key={sev}
                type="button"
                onClick={() => onSeverityChange(sev)}
                className={cn(
                  'flex-1 cursor-pointer rounded-md border px-2 py-1 text-[11px] font-medium transition-colors',
                  selectedSeverity === sev
                    ? 'border-foreground/20 bg-foreground/10 text-foreground'
                    : 'border-border-subtle bg-surface-elevated text-muted-foreground hover:text-foreground',
                )}
              >
                {sev}
              </button>
            ))}
          </div>
        </section>
      </div>
    </SidebarShell>
  )
}
