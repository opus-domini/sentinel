import { AlertTriangle, CheckCircle2, CircleOff, Pin, RefreshCw } from 'lucide-react'
import type { OpsBrowsedService } from '@/types'
import { TooltipHelper } from '@/components/TooltipHelper'
import {
  formatOpsUnitName,
  isOpsServiceActive,
  isOpsServiceChanging,
  isOpsServiceFailed,
  isOpsServiceInactive,
} from '@/lib/opsServices'
import type { OpsServiceStateFilter, OpsServiceTrackFilter } from '@/lib/opsServices'
import { cn } from '@/lib/utils'

type ServicesOperationsSummaryProps = {
  services: Array<OpsBrowsedService>
  trackingServices: Array<OpsBrowsedService>
  stateFilter: OpsServiceStateFilter
  trackFilter: OpsServiceTrackFilter
  onStateFilterChange: (filter: OpsServiceStateFilter) => void
  onTrackFilterChange: (filter: OpsServiceTrackFilter) => void
}

function firstServiceLabel(services: Array<OpsBrowsedService>): string | null {
  const first = services[0]
  if (!first) return null
  return first.description && first.description !== first.unit
    ? first.description
    : formatOpsUnitName(first.unit)
}

export function ServicesOperationsSummary({
  services,
  trackingServices,
  stateFilter,
  trackFilter,
  onStateFilterChange,
  onTrackFilterChange,
}: ServicesOperationsSummaryProps) {
  const failed = services.filter(isOpsServiceFailed)
  const changing = services.filter(isOpsServiceChanging)
  const active = services.filter(isOpsServiceActive)
  const inactive = services.filter(isOpsServiceInactive)
  const tracked = trackingServices.filter((service) => service.tracked)

  const items = [
    {
      label: 'Failed units',
      shortLabel: 'Failed',
      value: failed.length,
      detail: firstServiceLabel(failed) ?? 'No failed units',
      icon: AlertTriangle,
      selected: stateFilter === 'failed',
      onClick: () => onStateFilterChange('failed'),
      className: failed.length > 0 ? 'text-destructive-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Changing units',
      shortLabel: 'Changing',
      value: changing.length,
      detail: firstServiceLabel(changing) ?? 'No units changing state',
      icon: RefreshCw,
      selected: stateFilter === 'changing',
      onClick: () => onStateFilterChange('changing'),
      className: changing.length > 0 ? 'text-warning-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Active units',
      shortLabel: 'Active',
      value: active.length,
      detail: firstServiceLabel(active) ?? 'No active units',
      icon: CheckCircle2,
      selected: stateFilter === 'active',
      onClick: () => onStateFilterChange('active'),
      className: active.length > 0 ? 'text-ok-foreground' : 'text-muted-foreground',
    },
    {
      label: 'Stopped units',
      shortLabel: 'Stopped',
      value: inactive.length,
      detail: firstServiceLabel(inactive) ?? 'No stopped units',
      icon: CircleOff,
      selected: stateFilter === 'inactive',
      onClick: () => onStateFilterChange('inactive'),
      className: 'text-muted-foreground',
    },
    {
      label: 'Pinned units',
      shortLabel: 'Pinned',
      value: tracked.length,
      detail: firstServiceLabel(tracked) ?? 'No pinned units',
      icon: Pin,
      selected: trackFilter === 'tracked',
      onClick: () => onTrackFilterChange('tracked'),
      className: tracked.length > 0 ? 'text-primary-text-bright' : 'text-muted-foreground',
    },
  ]

  return (
    <section className="grid min-w-0 grid-cols-5 gap-1.5" aria-label="Services operations summary">
      {items.map((item) => {
        const Icon = item.icon
        const isActionable = item.value > 0 || item.selected
        return (
          <TooltipHelper key={item.label} side="bottom" content={`${item.label}\n${item.detail}`}>
            <button
              type="button"
              aria-disabled={!isActionable}
              tabIndex={isActionable ? 0 : -1}
              onClick={() => {
                if (isActionable) item.onClick()
              }}
              className={cn(
                'group h-10 min-w-0 overflow-hidden rounded-md border border-border-subtle bg-secondary px-1.5 py-1 text-left transition-colors sm:h-12 sm:px-2 sm:py-1.5',
                isActionable &&
                  'cursor-pointer hover:border-accent hover:bg-accent/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                item.selected && 'border-accent bg-accent/15',
                !isActionable && 'cursor-default opacity-75',
              )}
              aria-label={`${item.label}: ${item.value}. ${item.detail}`}
            >
              <div className="flex min-w-0 items-center gap-1 sm:gap-1.5">
                <Icon className={cn('h-3.5 w-3.5 shrink-0', item.className)} />
                <span className="hidden min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-[0.06em] text-muted-foreground sm:block">
                  {item.shortLabel}
                </span>
                <span className={cn('shrink-0 text-[13px] font-semibold', item.className)}>
                  {item.value}
                </span>
              </div>
              <p className="mt-0.5 hidden min-w-0 truncate text-[10px] text-muted-foreground sm:block">
                {item.detail}
              </p>
            </button>
          </TooltipHelper>
        )
      })}
    </section>
  )
}
