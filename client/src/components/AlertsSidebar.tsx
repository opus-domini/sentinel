import { useMemo, useState } from 'react'
import { Lock, LockOpen } from 'lucide-react'
import type { OpsAlert } from '@/types'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type AlertsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  token: string
  loading: boolean
  alerts: Array<OpsAlert>
  onTokenChange: (value: string) => void
  onAckAlert: (id: number) => void
}

function severityDot(severity: string): string {
  const s = severity.trim().toLowerCase()
  if (s === 'error') return 'bg-red-500'
  if (s === 'warn' || s === 'warning') return 'bg-amber-500'
  return 'bg-muted-foreground/50'
}

export default function AlertsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  token,
  loading,
  alerts,
  onTokenChange,
  onAckAlert,
}: AlertsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const hasToken = token.trim() !== ''

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return hasToken ? 'Token configured (required)' : 'Token required'
    }
    return hasToken ? 'Token configured' : 'No token'
  }, [hasToken, tokenRequired])

  const openAlerts = useMemo(
    () => alerts.filter((a) => a.status === 'open'),
    [alerts],
  )

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Alerts
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {alerts.length}
            </span>
            {openAlerts.length > 0 && (
              <span className="rounded-full bg-amber-500/20 px-1.5 text-[10px] text-amber-200">
                {openAlerts.length} open
              </span>
            )}
            <div className="ml-auto flex items-center gap-1.5">
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

        <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <div className="flex items-center justify-between border-b border-border-subtle px-2 py-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Recent Alerts
            </span>
            <span className="text-[10px] text-muted-foreground">
              {loading ? 'syncing...' : `${alerts.length} total`}
            </span>
          </div>
          <ScrollArea className="h-full min-h-0">
            <div className="grid min-h-0 gap-1.5 p-2">
              {loading && alerts.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  Loading alerts...
                </EmptyState>
              )}

              {!loading && alerts.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="grid gap-1 border-dashed p-3 text-[12px]"
                >
                  <span>No active alerts.</span>
                </EmptyState>
              )}

              {alerts.map((alert) => (
                <div
                  key={alert.id}
                  className={cn(
                    'grid min-w-0 gap-1 overflow-hidden rounded border px-2 py-1.5 text-[12px]',
                    alert.severity === 'error'
                      ? 'border-red-500/30 bg-red-500/5'
                      : 'border-amber-500/30 bg-amber-500/5',
                  )}
                >
                  <div className="flex min-w-0 items-center gap-1.5">
                    <span
                      className={cn(
                        'h-2 w-2 shrink-0 rounded-full',
                        severityDot(alert.severity),
                      )}
                    />
                    <span className="min-w-0 flex-1 truncate font-semibold">
                      {alert.title}
                    </span>
                    <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1 text-[9px] text-muted-foreground">
                      {alert.status}
                    </span>
                  </div>
                  <p className="truncate text-[10px] text-muted-foreground">
                    {alert.resource} · {alert.occurrences}x
                  </p>
                  {alert.status === 'open' && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-6 w-fit cursor-pointer px-2 text-[10px]"
                      onClick={() => onAckAlert(alert.id)}
                    >
                      Ack
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </ScrollArea>
        </section>
      </div>
    </SidebarShell>
  )
}
