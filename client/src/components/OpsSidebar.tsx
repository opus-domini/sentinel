import { useMemo, useState } from 'react'
import { RefreshCw, Wrench } from 'lucide-react'
import type { OpsServiceStatus } from '@/types'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type OpsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  token: string
  loading: boolean
  error: string
  services: Array<OpsServiceStatus>
  onTokenChange: (value: string) => void
  onRefresh: () => void
}

function serviceTone(service: OpsServiceStatus): string {
  const state = service.activeState.trim().toLowerCase()
  if (state === 'active' || state === 'running') {
    return 'border-emerald-500/45 bg-emerald-500/10 text-emerald-100'
  }
  if (state === 'failed') {
    return 'border-red-500/45 bg-red-500/10 text-red-100'
  }
  return 'border-border-subtle bg-surface-elevated text-secondary-foreground'
}

export default function OpsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  token,
  loading,
  error,
  services,
  onTokenChange,
  onRefresh,
}: OpsSidebarProps) {
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
      <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Ops
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {services.length}
            </span>
            <div className="ml-auto flex items-center gap-1.5">
              <TooltipHelper content="Refresh services">
                <Button
                  variant="ghost"
                  size="icon"
                  className="border border-border bg-surface-hover text-foreground hover:bg-accent"
                  onClick={onRefresh}
                  aria-label="Refresh services"
                >
                  <RefreshCw className="h-4 w-4" />
                </Button>
              </TooltipHelper>
              <TooltipHelper content={lockLabel}>
                <Button
                  variant="ghost"
                  size="icon"
                  className="border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => setIsTokenOpen(true)}
                  aria-label="API token"
                >
                  <Wrench className="h-4 w-4" />
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

        <section className="min-h-0 overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <ScrollArea className="h-full">
            <div className="grid min-h-0 gap-1.5 p-2">
              {loading && services.length === 0 && (
                <EmptyState variant="inline" className="border-dashed text-[12px]">
                  Loading services...
                </EmptyState>
              )}

              {!loading && services.length === 0 && (
                <EmptyState variant="inline" className="border-dashed text-[12px]">
                  No managed services found.
                </EmptyState>
              )}

              {services.map((service) => (
                <div
                  key={service.name}
                  className={cn(
                    'grid gap-1 rounded border px-2 py-1.5 text-[12px]',
                    serviceTone(service),
                  )}
                >
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <span className="min-w-0 truncate font-semibold">
                      {service.displayName}
                    </span>
                    <span className="shrink-0 rounded border border-border-subtle bg-surface-overlay px-1 text-[10px] text-muted-foreground">
                      {service.scope}
                    </span>
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    {service.unit}
                  </div>
                </div>
              ))}

              {error !== '' && (
                <p className="mt-1 text-[12px] text-destructive-foreground">
                  {error}
                </p>
              )}
            </div>
          </ScrollArea>
        </section>
      </div>
    </SidebarShell>
  )
}
