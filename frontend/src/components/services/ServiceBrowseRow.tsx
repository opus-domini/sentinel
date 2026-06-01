import { memo, useState } from 'react'
import {
  Clock3,
  FileText,
  Pin,
  PinOff,
  Play,
  Power,
  PowerOff,
  RotateCw,
  Square,
} from 'lucide-react'
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

  return (
    <>
      <div className="grid min-w-0 gap-1.5 rounded border border-border-subtle bg-surface-elevated px-2 py-1.5">
        <div className="flex min-w-0 items-start gap-2">
          <span
            className={cn('mt-1 h-2 w-2 shrink-0 rounded-full', browsedServiceDot(svc.activeState))}
          />
          <div className="min-w-0 flex-1">
            <div className="grid min-w-0 gap-1 sm:flex sm:items-center sm:gap-1.5">
              <p
                className="min-w-0 flex-1 truncate text-[12px] font-medium"
                title={unitLabel === svc.unit ? undefined : svc.unit}
              >
                {unitLabel}
              </p>
              <div className="flex min-w-0 flex-wrap items-center gap-1 sm:shrink-0">
                <TooltipHelper content="Unit type discovered on the host">
                  <span className="cursor-default rounded border border-border-subtle px-1 text-[9px] text-muted-foreground">
                    {svc.unitType}
                  </span>
                </TooltipHelper>
                <TooltipHelper content="Unit scope (user or system)">
                  <span className="cursor-default rounded border border-border-subtle px-1 text-[9px] text-muted-foreground">
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
        <div className="grid min-w-0 gap-1 pl-4 sm:flex sm:items-center sm:gap-2">
          <TooltipHelper content={description || svc.unit}>
            <p className="min-w-0 flex-1 truncate text-[10px] text-muted-foreground">
              {description || svc.unit}
            </p>
          </TooltipHelper>
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
            <TooltipHelper content="Start service">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => handleAction('start')}
                disabled={startDisabled}
                aria-label="Start service"
              >
                <Play className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Stop service">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => handleAction('stop')}
                disabled={stopDisabled}
                aria-label="Stop service"
              >
                <Square className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Restart service">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => handleAction('restart')}
                disabled={rowBusy}
                aria-label="Restart service"
              >
                <RotateCw className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Enable service">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => handleAction('enable')}
                disabled={enableDisabled}
                aria-label="Enable service"
              >
                <Power className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Disable service">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => handleAction('disable')}
                disabled={disableDisabled}
                aria-label="Disable service"
              >
                <PowerOff className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="Inspect status">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => onInspect(svc)}
                disabled={rowBusy}
                aria-label="Inspect service status"
              >
                <FileText className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content="View logs">
              <Button
                variant="outline"
                size="icon-lg"
                className="h-8 w-8 cursor-pointer sm:h-6 sm:w-6"
                onClick={() => onLogs(svc)}
                disabled={rowBusy}
                aria-label="View service logs"
              >
                <Clock3 className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
              </Button>
            </TooltipHelper>
            <TooltipHelper content={svc.tracked ? 'Unpin from sidebar' : 'Pin to sidebar'}>
              <Button
                variant="outline"
                size="icon-lg"
                className={cn(
                  'h-8 w-8 cursor-pointer sm:h-6 sm:w-6',
                  svc.tracked ? 'text-primary-text-bright' : '',
                )}
                onClick={() => onToggleTrack(svc)}
                disabled={rowBusy}
                aria-label={svc.tracked ? 'Unpin service' : 'Pin service'}
              >
                {svc.tracked ? (
                  <PinOff className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
                ) : (
                  <Pin className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
                )}
              </Button>
            </TooltipHelper>
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
