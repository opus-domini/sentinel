import { memo, useState } from 'react'
import {
  Clock3,
  FileText,
  MoreHorizontal,
  Pin,
  PinOff,
  Play,
  Power,
  PowerOff,
  RotateCw,
  Square,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { OpsBrowsedService, OpsServiceAction } from '@/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { TooltipHelper } from '@/components/TooltipHelper'
import { canStartOpsService, canStopOpsService, formatOpsUnitName } from '@/lib/opsServices'
import { browsedServiceDot } from '@/lib/opsUtils'
import { cn } from '@/lib/utils'

function canEnableService(svc: OpsBrowsedService): boolean {
  const state = svc.enabledState.trim().toLowerCase()
  return state === 'disabled' || state === 'masked'
}

function canDisableService(svc: OpsBrowsedService): boolean {
  const state = svc.enabledState.trim().toLowerCase()
  return state === 'enabled' || state === 'static'
}

function requiresConfirmation(svc: OpsBrowsedService, action: OpsServiceAction): boolean {
  return (
    svc.scope === 'system' && (action === 'stop' || action === 'restart' || action === 'disable')
  )
}

function actionLabel(action: OpsServiceAction): string {
  return action[0].toUpperCase() + action.slice(1)
}

type RowAction = {
  key: string
  label: string
  icon: LucideIcon
  onClick: () => void
  disabled: boolean
  destructive?: boolean
  highlight?: boolean
}

type ServiceBrowseRowProps = {
  service: OpsBrowsedService
  pendingAction: OpsServiceAction | undefined
  onAction: (svc: OpsBrowsedService, action: OpsServiceAction) => void
  onInspect: (svc: OpsBrowsedService) => void
  onLogs: (svc: OpsBrowsedService) => void
  onToggleTrack: (svc: OpsBrowsedService) => void
}

export const ServiceBrowseRow = memo(function ServiceBrowseRow({
  service: svc,
  pendingAction: pending,
  onAction,
  onInspect,
  onLogs,
  onToggleTrack,
}: ServiceBrowseRowProps) {
  const [confirmAction, setConfirmAction] = useState<OpsServiceAction>()
  const rowBusy = pending !== undefined
  const startDisabled = rowBusy || !canStartOpsService(svc)
  const stopDisabled = rowBusy || !canStopOpsService(svc)
  const enableDisabled = rowBusy || !canEnableService(svc)
  const disableDisabled = rowBusy || !canDisableService(svc)
  const unitLabel = formatOpsUnitName(svc.unit)
  const description =
    svc.description && svc.description !== svc.unit && svc.description !== unitLabel
      ? svc.description
      : ''
  const handleAction = (action: OpsServiceAction) => {
    if (requiresConfirmation(svc, action)) {
      setConfirmAction(action)
      return
    }

    onAction(svc, action)
  }

  const actions: Array<RowAction> = [
    {
      key: 'start',
      label: 'Start',
      icon: Play,
      onClick: () => handleAction('start'),
      disabled: startDisabled,
    },
    {
      key: 'stop',
      label: 'Stop',
      icon: Square,
      onClick: () => handleAction('stop'),
      disabled: stopDisabled,
      destructive: true,
    },
    {
      key: 'restart',
      label: 'Restart',
      icon: RotateCw,
      onClick: () => handleAction('restart'),
      disabled: rowBusy,
    },
    {
      key: 'enable',
      label: 'Enable',
      icon: Power,
      onClick: () => handleAction('enable'),
      disabled: enableDisabled,
    },
    {
      key: 'disable',
      label: 'Disable',
      icon: PowerOff,
      onClick: () => handleAction('disable'),
      disabled: disableDisabled,
      destructive: true,
    },
    {
      key: 'inspect',
      label: 'Inspect status',
      icon: FileText,
      onClick: () => onInspect(svc),
      disabled: rowBusy,
    },
    {
      key: 'logs',
      label: 'View logs',
      icon: Clock3,
      onClick: () => onLogs(svc),
      disabled: rowBusy,
    },
    {
      key: 'track',
      label: svc.tracked ? 'Unpin service' : 'Pin service',
      icon: svc.tracked ? PinOff : Pin,
      onClick: () => onToggleTrack(svc),
      disabled: rowBusy,
      highlight: svc.tracked,
    },
  ]

  // On mobile the most relevant single action stays inline; the rest collapse
  // into an overflow menu so each card is short and many units fit on screen.
  const primary = !startDisabled ? actions[0] : !stopDisabled ? actions[1] : undefined

  return (
    <>
      <div className="grid min-w-0 gap-1.5 rounded border border-border-subtle bg-surface-elevated px-2 py-1.5">
        <div className="flex min-w-0 items-start gap-2">
          <span
            className={cn('mt-1 h-2 w-2 shrink-0 rounded-full', browsedServiceDot(svc.activeState))}
          />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-1.5">
              <p
                className="min-w-0 flex-1 truncate text-[12px] font-medium"
                title={unitLabel === svc.unit ? undefined : svc.unit}
              >
                {unitLabel}
              </p>
              <div className="flex shrink-0 items-center gap-1">
                <TooltipHelper content="Unit type discovered on the host">
                  <span className="hidden cursor-default rounded border border-border-subtle px-1 text-[9px] text-muted-foreground sm:inline-block">
                    {svc.unitType}
                  </span>
                </TooltipHelper>
                <TooltipHelper content="Unit scope (user or system)">
                  <span className="hidden cursor-default rounded border border-border-subtle px-1 text-[9px] text-muted-foreground sm:inline-block">
                    {svc.scope}
                  </span>
                </TooltipHelper>
                <TooltipHelper content="Runtime state (active, inactive, failed, …)">
                  <span
                    className={cn(
                      'cursor-default rounded border px-1 text-[9px]',
                      svc.activeState === 'active' || svc.activeState === 'running'
                        ? 'border-ok/30 text-ok-foreground'
                        : svc.activeState === 'failed'
                          ? 'border-destructive/30 text-destructive-foreground'
                          : 'border-border-subtle text-muted-foreground',
                    )}
                  >
                    {svc.activeState}
                  </span>
                </TooltipHelper>
                <TooltipHelper content="Boot state (enabled starts on boot, disabled does not)">
                  <span
                    className={cn(
                      'cursor-default rounded border px-1 text-[9px]',
                      svc.enabledState === 'enabled'
                        ? 'border-ok/30 text-ok-foreground'
                        : svc.enabledState === 'disabled'
                          ? 'border-warning/30 text-warning-foreground'
                          : 'border-border-subtle text-muted-foreground',
                    )}
                  >
                    {svc.enabledState || 'unknown'}
                  </span>
                </TooltipHelper>
              </div>
            </div>
          </div>
          {pending && (
            <span className="shrink-0 text-[10px] text-muted-foreground">{pending}...</span>
          )}
        </div>
        <div className="flex min-w-0 items-center gap-2 pl-4">
          <TooltipHelper content={description || svc.unit}>
            <p className="min-w-0 flex-1 truncate text-[10px] text-muted-foreground">
              {description || svc.unit}
            </p>
          </TooltipHelper>
          {/* Desktop: full inline icon row. */}
          <div
            data-testid="service-actions-desktop"
            className="hidden shrink-0 flex-wrap items-center justify-end gap-1 sm:flex"
          >
            {actions.map((a) => {
              const Icon = a.icon
              return (
                <TooltipHelper key={a.key} content={a.label}>
                  <Button
                    variant="outline"
                    size="icon-sm"
                    className={cn(
                      'h-6 w-6 cursor-pointer',
                      a.highlight && 'text-primary-text-bright',
                    )}
                    onClick={a.onClick}
                    disabled={a.disabled}
                    aria-label={a.label}
                  >
                    <Icon className="h-3 w-3" />
                  </Button>
                </TooltipHelper>
              )
            })}
          </div>
          {/* Mobile: one primary action inline, everything else in an overflow
              menu so each card stays short and many units fit on screen. */}
          <div
            data-testid="service-actions-mobile"
            className="flex shrink-0 items-center gap-1 sm:hidden"
          >
            {primary && (
              <Button
                variant="outline"
                size="icon-lg"
                className="h-9 w-9 cursor-pointer"
                onClick={primary.onClick}
                disabled={primary.disabled}
                aria-label={primary.label}
              >
                {(() => {
                  const Icon = primary.icon
                  return <Icon className="h-4 w-4" />
                })()}
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="outline"
                  size="icon-lg"
                  className="h-9 w-9 cursor-pointer"
                  disabled={rowBusy}
                  aria-label="More actions"
                >
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-44">
                {actions.map((a) => {
                  const Icon = a.icon
                  return (
                    <DropdownMenuItem
                      key={a.key}
                      disabled={a.disabled}
                      variant={a.destructive ? 'destructive' : 'default'}
                      onSelect={() => a.onClick()}
                      className={cn('min-h-9', a.highlight && 'text-primary-text-bright')}
                    >
                      <Icon className="h-3.5 w-3.5" />
                      {a.label}
                    </DropdownMenuItem>
                  )
                })}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
      <AlertDialog
        open={confirmAction !== undefined}
        onOpenChange={(open) => {
          if (!open) setConfirmAction(undefined)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Confirm {confirmAction ? actionLabel(confirmAction).toLowerCase() : 'action'}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmAction
                ? `${actionLabel(confirmAction)} ${unitLabel}? This affects a system service.`
                : 'Confirm this system service action.'}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <dl className="grid gap-1 rounded border border-border-subtle bg-surface px-2 py-2 text-[11px]">
            <div className="grid grid-cols-[4.5rem_1fr] gap-2">
              <dt className="text-muted-foreground">Action</dt>
              <dd>{confirmAction ? actionLabel(confirmAction) : 'Action'}</dd>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-2">
              <dt className="text-muted-foreground">Service</dt>
              <dd className="min-w-0 truncate">{unitLabel}</dd>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-2">
              <dt className="text-muted-foreground">Manager</dt>
              <dd>{svc.manager}</dd>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-2">
              <dt className="text-muted-foreground">Scope</dt>
              <dd>{svc.scope}</dd>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-2">
              <dt className="text-muted-foreground">Unit</dt>
              <dd className="min-w-0 truncate">{svc.unit}</dd>
            </div>
          </dl>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (confirmAction) onAction(svc, confirmAction)
                setConfirmAction(undefined)
              }}
            >
              Confirm
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
})

ServiceBrowseRow.displayName = 'ServiceBrowseRow'
