import { useMemo, useState } from 'react'
import { Lock, LockOpen, Trash2 } from 'lucide-react'
import type { OpsServiceStatus } from '@/types'
import ServicesHelpDialog from '@/components/ServicesHelpDialog'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'
import { filterOpsServicesByQuery } from '@/lib/opsServices'

type ServicesSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  loading: boolean
  error: string
  services: Array<OpsServiceStatus>
  onTokenChange: (value: string) => void
  onRemoveService: (name: string) => Promise<void>
  onNavigateToService: (unit: string) => void
}

function statusDot(service: OpsServiceStatus): string {
  const state = service.activeState.trim().toLowerCase()
  if (state === 'active' || state === 'running') return 'bg-ok'
  if (state === 'failed') return 'bg-destructive'
  return 'bg-muted-foreground/50'
}

export default function ServicesSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  authenticated,
  loading,
  error,
  services,
  onTokenChange,
  onRemoveService,
  onNavigateToService,
}: ServicesSidebarProps) {
  const { sidebarDensity } = useLayoutContext()
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const [filter, setFilter] = useState('')
  const debouncedFilter = useDebouncedValue(filter)
  const [removing, setRemoving] = useState<string | null>(null)
  const filteredServices = useMemo(
    () => filterOpsServicesByQuery(services, debouncedFilter),
    [services, debouncedFilter],
  )
  const hasFilter = filter.trim() !== ''

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

  async function handleRemove(name: string) {
    setRemoving(name)
    try {
      await onRemoveService(name)
    } finally {
      setRemoving(null)
    }
  }

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Services
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {services.length}
            </span>
            <div className="ml-auto flex items-center gap-1">
              <ServicesHelpDialog />
              <TooltipHelper content={lockLabel}>
                <Button
                  variant="outline"
                  size="icon-xs"
                  className="cursor-pointer text-secondary-foreground"
                  onClick={() => setIsTokenOpen(true)}
                  aria-label="API token"
                >
                  {authenticated ? (
                    <Lock className="h-3 w-3" />
                  ) : (
                    <LockOpen className="h-3 w-3" />
                  )}
                </Button>
              </TooltipHelper>
            </div>
          </div>
          <Input
            className="bg-surface-overlay text-[12px] md:h-8"
            name="services-filter"
            placeholder="filter services..."
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />

          <TokenDialog
            open={isTokenOpen}
            onOpenChange={setIsTokenOpen}
            authenticated={authenticated}
            onTokenChange={onTokenChange}
            tokenRequired={tokenRequired}
          />
        </section>

        <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <ScrollArea className="h-full min-h-0">
            <div className="grid min-h-0 gap-1.5 p-2">
              <div className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                Pinned
              </div>
              {loading && services.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  Loading services...
                </EmptyState>
              )}

              {!loading && filteredServices.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="grid gap-1 border-dashed p-3 text-[12px]"
                >
                  <span>
                    {hasFilter
                      ? 'No tracked services match filter.'
                      : 'No tracked services. Use the browse panel to pin services.'}
                  </span>
                  {hasFilter && (
                    <Button
                      variant="outline"
                      className="mx-auto h-7 px-2 text-[11px]"
                      type="button"
                      onClick={() => setFilter('')}
                    >
                      Clear Filter
                    </Button>
                  )}
                </EmptyState>
              )}

              {filteredServices.map((service) => (
                <div
                  key={service.name}
                  className={cn(
                    'grid min-w-0 overflow-hidden rounded border border-border-subtle',
                    sidebarDensity === 'minimal'
                      ? 'px-1.5 py-1 text-[11px]'
                      : sidebarDensity === 'compact'
                        ? 'gap-1 px-2 py-1.5 text-[12px]'
                        : 'gap-1.5 px-2.5 py-2 text-[12px]',
                  )}
                >
                  {sidebarDensity === 'minimal' ? (
                    <div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
                      <span
                        className={cn(
                          'h-1.5 w-1.5 shrink-0 rounded-full',
                          statusDot(service),
                        )}
                      />
                      <button
                        type="button"
                        className="block w-full min-w-0 flex-1 cursor-pointer overflow-hidden text-ellipsis whitespace-nowrap text-left font-medium hover:text-primary-text-bright"
                        onClick={() => onNavigateToService(service.unit)}
                      >
                        {service.displayName}
                      </button>
                    </div>
                  ) : (
                    <>
                      <div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
                        <span
                          className={cn(
                            'h-2 w-2 shrink-0 rounded-full',
                            statusDot(service),
                          )}
                        />
                        <TooltipHelper
                          content={`${service.displayName} (${service.unit})`}
                        >
                          <button
                            type="button"
                            className="block w-full min-w-0 flex-1 cursor-pointer overflow-hidden text-ellipsis whitespace-nowrap text-left font-semibold hover:text-primary-text-bright"
                            onClick={() => onNavigateToService(service.unit)}
                          >
                            {service.displayName}
                          </button>
                        </TooltipHelper>
                        <TooltipHelper content="Unpin service">
                          <button
                            type="button"
                            className="inline-flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded text-muted-foreground hover:bg-destructive/20 hover:text-destructive-foreground"
                            onClick={() => void handleRemove(service.name)}
                            disabled={removing === service.name}
                            aria-label={`Unpin ${service.displayName}`}
                          >
                            <Trash2 className="h-3 w-3" />
                          </button>
                        </TooltipHelper>
                      </div>
                      <div className="flex min-w-0 items-center justify-between gap-1.5 text-[10px] text-muted-foreground">
                        <TooltipHelper content={service.unit}>
                          <span className="min-w-0 flex-1 truncate">
                            {service.unit}
                          </span>
                        </TooltipHelper>
                        {sidebarDensity === 'full' && (
                          <span className="shrink-0 rounded border border-border-subtle bg-surface-overlay px-1 text-[10px] text-muted-foreground">
                            {service.scope}
                          </span>
                        )}
                      </div>
                    </>
                  )}
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
