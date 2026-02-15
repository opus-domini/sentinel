import { useCallback, useMemo, useState } from 'react'
import {
  CircleHelp,
  Loader2,
  Lock,
  LockOpen,
  Plus,
  Search,
  Trash2,
} from 'lucide-react'
import type { OpsAvailableService, OpsServiceStatus } from '@/types'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  onDiscoverServices: () => Promise<Array<OpsAvailableService>>
  onAddService: (svc: OpsAvailableService) => Promise<void>
  onRemoveService: (name: string) => Promise<void>
}

function serviceTone(service: OpsServiceStatus): string {
  const state = service.activeState.trim().toLowerCase()
  if (state === 'active' || state === 'running') {
    return 'border-emerald-500/45'
  }
  if (state === 'failed') {
    return 'border-red-500/45'
  }
  return 'border-border-subtle'
}

function availableStateBadge(state: string): string {
  const s = state.trim().toLowerCase()
  if (s === 'active' || s === 'running') {
    return 'border-emerald-500/40 text-emerald-300'
  }
  if (s === 'failed') {
    return 'border-red-500/40 text-red-300'
  }
  return 'border-border-subtle text-muted-foreground'
}

/** Names reserved by the backend (built-in services). */
const builtinNames = new Set(['sentinel', 'sentinel-updater'])

export default function OpsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  token,
  loading,
  error,
  services,
  onTokenChange,
  onDiscoverServices,
  onAddService,
  onRemoveService,
}: OpsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const [isPickerOpen, setIsPickerOpen] = useState(false)
  const [available, setAvailable] = useState<Array<OpsAvailableService>>([])
  const [discovering, setDiscovering] = useState(false)
  const [filter, setFilter] = useState('')
  const [adding, setAdding] = useState<string | null>(null)
  const [removing, setRemoving] = useState<string | null>(null)
  const hasToken = token.trim() !== ''

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return hasToken ? 'Token configured (required)' : 'Token required'
    }
    return hasToken ? 'Token configured' : 'No token'
  }, [hasToken, tokenRequired])

  const openPicker = useCallback(async () => {
    setIsPickerOpen(true)
    setFilter('')
    setDiscovering(true)
    try {
      const result = await onDiscoverServices()
      setAvailable(result)
    } catch {
      setAvailable([])
    } finally {
      setDiscovering(false)
    }
  }, [onDiscoverServices])

  const filtered = useMemo(() => {
    if (filter.trim() === '') return available
    const q = filter.trim().toLowerCase()
    return available.filter(
      (s) =>
        s.unit.toLowerCase().includes(q) ||
        s.description.toLowerCase().includes(q),
    )
  }, [available, filter])

  async function handleAdd(svc: OpsAvailableService) {
    setAdding(svc.unit)
    try {
      await onAddService(svc)
      setAvailable((prev) => prev.filter((s) => s.unit !== svc.unit))
    } finally {
      setAdding(null)
    }
  }

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
      <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Managed Services
            </span>
            <TooltipHelper content="Services discovered on your host for quick status and actions.">
              <span
                className="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground"
                aria-label="About managed services"
              >
                <CircleHelp className="h-3 w-3 cursor-help" />
              </span>
            </TooltipHelper>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {services.length}
            </span>
            <div className="ml-auto flex items-center gap-1.5">
              <TooltipHelper content="Add service from host">
                <Button
                  variant="ghost"
                  size="icon"
                  className="cursor-pointer border border-border bg-surface-hover text-foreground hover:bg-accent"
                  onClick={() => void openPicker()}
                  aria-label="Add service from host"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </TooltipHelper>
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

          <Dialog open={isPickerOpen} onOpenChange={setIsPickerOpen}>
            <DialogContent className="max-h-[80vh] overflow-hidden sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Add service</DialogTitle>
                <DialogDescription>
                  Pick a service from your host to monitor.
                </DialogDescription>
              </DialogHeader>

              <div className="relative">
                <Search className="absolute left-2 top-2 h-4 w-4 text-muted-foreground" />
                <input
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  placeholder="Filter services..."
                  className="h-8 w-full rounded-md border border-border bg-surface-elevated pl-8 pr-2 text-[12px] placeholder:text-muted-foreground"
                  autoFocus
                />
              </div>

              <ScrollArea className="max-h-[50vh]">
                <div className="grid gap-1 pr-2">
                  {discovering && (
                    <div className="flex items-center justify-center gap-2 py-6 text-[12px] text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Discovering services...
                    </div>
                  )}

                  {!discovering && filtered.length === 0 && (
                    <EmptyState
                      variant="inline"
                      className="border-dashed text-[12px]"
                    >
                      {filter.trim() !== ''
                        ? 'No services match your filter.'
                        : 'No additional services found on this host.'}
                    </EmptyState>
                  )}

                  {!discovering &&
                    filtered.map((svc) => (
                      <button
                        key={`${svc.scope}:${svc.unit}`}
                        type="button"
                        className="flex w-full min-w-0 cursor-pointer items-center gap-2 overflow-hidden rounded border border-border-subtle bg-surface-elevated px-2.5 py-2 text-left transition-colors hover:border-primary/40 hover:bg-surface-hover disabled:cursor-wait disabled:opacity-60"
                        onClick={() => void handleAdd(svc)}
                        disabled={adding === svc.unit}
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-[12px] font-semibold">
                            {svc.unit}
                          </p>
                          {svc.description && svc.description !== svc.unit && (
                            <p className="truncate text-[10px] text-muted-foreground">
                              {svc.description}
                            </p>
                          )}
                        </div>
                        <div className="flex shrink-0 items-center gap-1.5">
                          <span className="rounded border px-1 text-[9px]">
                            {svc.scope}
                          </span>
                          <span
                            className={cn(
                              'rounded border px-1 text-[9px]',
                              availableStateBadge(svc.activeState),
                            )}
                          >
                            {svc.activeState}
                          </span>
                        </div>
                      </button>
                    ))}
                </div>
              </ScrollArea>
            </DialogContent>
          </Dialog>
        </section>

        <section className="min-h-0 overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <div className="flex items-center justify-between border-b border-border-subtle px-2 py-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Service Status
            </span>
            <span className="text-[10px] text-muted-foreground">
              {loading ? 'syncing...' : `${services.length} tracked`}
            </span>
          </div>
          <ScrollArea className="h-full">
            <div className="grid min-h-0 gap-1.5 p-2">
              {loading && services.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  Loading services...
                </EmptyState>
              )}

              {!loading && services.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
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
                    <TooltipHelper
                      content={`${service.displayName} (${service.unit})`}
                    >
                      <span className="min-w-0 truncate font-semibold">
                        {service.displayName}
                      </span>
                    </TooltipHelper>
                    <div className="flex shrink-0 items-center gap-1">
                      <span className="rounded border border-border-subtle bg-surface-overlay px-1 text-[10px] text-muted-foreground">
                        {service.scope}
                      </span>
                      {!builtinNames.has(service.name) && (
                        <TooltipHelper content="Remove custom service">
                          <button
                            type="button"
                            className="inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:bg-red-500/20 hover:text-red-300"
                            onClick={() => void handleRemove(service.name)}
                            disabled={removing === service.name}
                            aria-label={`Remove ${service.displayName}`}
                          >
                            <Trash2 className="h-3 w-3" />
                          </button>
                        </TooltipHelper>
                      )}
                    </div>
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
