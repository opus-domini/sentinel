import { Link } from '@tanstack/react-router'
import type { OpsServiceInspect, OpsServiceStatusResponse } from '@/types'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { runbookDefinitionSearch, runbookExecutionSearch } from '@/lib/deepLinks'
import { formatOpsUnitName } from '@/lib/opsServices'

type ServiceStatusDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  loading: boolean
  error: string
  data: OpsServiceInspect | null
  context: OpsServiceStatusResponse['context'] | null
  onViewLogs?: () => void
}

export function ServiceStatusDialog({
  open,
  onOpenChange,
  loading,
  error,
  data,
  context,
  onViewLogs,
}: ServiceStatusDialogProps) {
  const conditionRows =
    data == null
      ? []
      : [
          ['Active state', data.condition.activeState],
          ['Sub-state', data.condition.subState],
          ['Result', data.condition.result],
          [
            'Exit code',
            data.condition.exitCode == null ? undefined : String(data.condition.exitCode),
          ],
          [
            'Exit status',
            data.condition.exitStatus == null ? undefined : String(data.condition.exitStatus),
          ],
          ['Transitioned at', data.condition.transitionedAt],
        ].filter((row): row is [string, string] => row[1] != null && row[1] !== '')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-[calc(100vw-1rem)] overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {data?.service.unit ? formatOpsUnitName(data.service.unit) : 'Service status'}
          </DialogTitle>
          <DialogDescription>
            {data?.summary ?? 'Runtime details from service manager'}
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 gap-2 overflow-hidden">
          {loading && (
            <p className="text-[12px] text-muted-foreground">Loading service status...</p>
          )}
          {error !== '' && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 px-2 py-1 text-[12px] text-destructive-foreground">
              {error}
            </p>
          )}

          {!loading && data != null && (
            <ScrollArea className="max-h-[58vh] min-h-0">
              <div className="grid gap-2 pr-2">
                <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-[11px] font-semibold text-foreground">
                        {formatOpsUnitName(data.service.unit)}
                      </p>
                      <p className="text-[10px] text-muted-foreground">
                        observed at {data.observedAt}
                      </p>
                    </div>
                    {onViewLogs && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-7 shrink-0 cursor-pointer text-[10px]"
                        onClick={onViewLogs}
                      >
                        View logs
                      </Button>
                    )}
                  </div>
                </div>

                <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                  <p className="mb-1 text-[11px] font-semibold text-foreground">
                    Current condition
                  </p>
                  {conditionRows.length > 0 ? (
                    <div className="grid gap-1 text-[11px]">
                      {conditionRows.map(([label, value]) => (
                        <div
                          key={label}
                          className="grid grid-cols-[6.5rem_1fr] gap-2 sm:grid-cols-[9rem_1fr]"
                        >
                          <span className="text-muted-foreground">{label}</span>
                          <span className="break-all font-mono text-foreground">{value}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">
                      No structured condition was reported.
                    </p>
                  )}
                </div>

                <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                  <p className="mb-1 text-[11px] font-semibold text-foreground">
                    Operational context
                  </p>
                  <div className="grid gap-1.5 text-[11px]">
                    {context?.runbook ? (
                      <Link
                        to="/runbooks"
                        search={runbookDefinitionSearch(context.runbook.id)}
                        className="rounded border border-primary/25 bg-primary/5 px-2 py-1.5 text-primary-text no-underline hover:bg-primary/10"
                      >
                        Procedure · {context.runbook.name}
                      </Link>
                    ) : (
                      <p className="text-muted-foreground">No procedure is associated.</p>
                    )}
                    {context?.latestRun ? (
                      <Link
                        to="/runbooks"
                        search={runbookExecutionSearch(context.latestRun.id)}
                        className="rounded border border-border-subtle bg-background px-2 py-1.5 text-foreground no-underline hover:bg-surface-hover"
                      >
                        Latest execution · {context.latestRun.status} ·{' '}
                        {context.latestRun.runbookName}
                      </Link>
                    ) : (
                      <p className="text-muted-foreground">
                        No execution receipt exists for this target.
                      </p>
                    )}
                  </div>
                </div>

                {data.properties != null && Object.keys(data.properties).length > 0 && (
                  <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                    <p className="mb-1 text-[11px] font-semibold text-foreground">Properties</p>
                    <div className="grid gap-1 overflow-hidden text-[11px]">
                      {Object.entries(data.properties)
                        .sort(([a], [b]) => a.localeCompare(b))
                        .map(([key, value]) => (
                          <div
                            key={key}
                            className="grid grid-cols-[5.5rem_1fr] gap-2 sm:grid-cols-[9rem_1fr]"
                          >
                            <span className="break-all font-mono text-muted-foreground">{key}</span>
                            <span className="break-all font-mono text-foreground">{value}</span>
                          </div>
                        ))}
                    </div>
                  </div>
                )}

                {data.output?.trim() !== '' && (
                  <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                    <p className="mb-1 text-[11px] font-semibold text-foreground">Raw output</p>
                    <pre className="max-h-[36vh] overflow-auto whitespace-pre-wrap break-words rounded border border-border-subtle bg-background p-2 font-mono text-[11px] text-secondary-foreground">
                      {data.output}
                    </pre>
                  </div>
                )}
              </div>
            </ScrollArea>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
