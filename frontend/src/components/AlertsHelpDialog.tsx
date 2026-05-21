import { useState } from 'react'
import { CircleHelp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TooltipHelper } from '@/components/TooltipHelper'

export default function AlertsHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Alerts">
        <Button
          variant="outline"
          size="icon-xs"
          className="cursor-pointer text-secondary-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Alerts"
        >
          <CircleHelp className="h-3 w-3" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Alerts</DialogTitle>
            <DialogDescription>
              How the Sentinel alerting system works.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">Sources</h3>
              <ul className="list-inside list-disc space-y-1">
                <li>
                  <strong className="text-foreground">health</strong> — service
                  failures and host resource alerts (CPU, memory, disk)
                </li>
                <li>
                  <strong className="text-foreground">service</strong> — service
                  state changes detected through the ops control plane
                </li>
                <li>
                  <strong className="text-foreground">runbook</strong> — runbook
                  execution failures and timeouts
                </li>
              </ul>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Severity</h3>
              <p>
                <strong className="text-warning-foreground">warn</strong> —
                degraded state or unusual activity that may need attention.
              </p>
              <p className="mt-1">
                <strong className="text-destructive-foreground">error</strong> —
                a failure that requires immediate action (e.g. a service crash).
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                Acknowledging &amp; Dismissing
              </h3>
              <p>
                Click <strong className="text-foreground">Ack</strong> on an
                open alert to mark it as acknowledged. Acked alerts remain
                visible but stop contributing to the open count. Alerts are
                auto-resolved when their underlying condition clears.
              </p>
              <p className="mt-1">
                Resolved alerts can be permanently dismissed via the{' '}
                <strong className="text-foreground">Dismiss</strong> button.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Limitations</h3>
              <p>
                Alerts are local to this Sentinel instance and are not forwarded
                to external systems. Duplicate alerts on the same resource are
                deduplicated by key and increment the occurrence counter.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
