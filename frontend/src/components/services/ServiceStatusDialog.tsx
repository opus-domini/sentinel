import type { OpsServiceInspect } from '@/types'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { formatOpsUnitName } from '@/lib/opsServices'

type ServiceStatusDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  loading: boolean
  error: string
  data: OpsServiceInspect | null
}

export function ServiceStatusDialog({
  open,
  onOpenChange,
  loading,
  error,
  data,
}: ServiceStatusDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-[calc(100vw-1rem)] overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {data?.service.unit
              ? formatOpsUnitName(data.service.unit)
              : 'Service status'}
          </DialogTitle>
          <DialogDescription>
            {data?.summary ?? 'Runtime details from service manager'}
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 gap-2 overflow-hidden">
          {loading && (
            <p className="text-[12px] text-muted-foreground">
              Loading service status...
            </p>
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
                  <p className="text-[11px] font-semibold text-foreground">
                    {formatOpsUnitName(data.service.unit)}
                  </p>
                  <p className="text-[10px] text-muted-foreground">
                    checked at {data.checkedAt}
                  </p>
                </div>

                {data.properties != null &&
                  Object.keys(data.properties).length > 0 && (
                    <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                      <p className="mb-1 text-[11px] font-semibold text-foreground">
                        Properties
                      </p>
                      <div className="grid gap-1 overflow-hidden text-[11px]">
                        {Object.entries(data.properties)
                          .sort(([a], [b]) => a.localeCompare(b))
                          .map(([key, value]) => (
                            <div
                              key={key}
                              className="grid grid-cols-[5.5rem_1fr] gap-2 sm:grid-cols-[9rem_1fr]"
                            >
                              <span className="break-all font-mono text-muted-foreground">
                                {key}
                              </span>
                              <span className="break-all font-mono text-foreground">
                                {value}
                              </span>
                            </div>
                          ))}
                      </div>
                    </div>
                  )}

                {data.output?.trim() !== '' && (
                  <div className="rounded-md border border-border-subtle bg-surface-overlay p-2">
                    <p className="mb-1 text-[11px] font-semibold text-foreground">
                      Raw output
                    </p>
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
