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

export default function MetricsHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Metrics">
        <Button
          variant="ghost"
          size="icon"
          className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Metrics"
        >
          <CircleHelp className="h-4 w-4" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Metrics</DialogTitle>
            <DialogDescription>
              How the Sentinel metrics dashboard works.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                System Metrics
              </h3>
              <p>
                CPU usage, memory consumption, disk utilization, and system load
                averages are collected from the host operating system.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                Runtime Metrics
              </h3>
              <p>
                Go runtime statistics including goroutine count and heap memory
                usage provide insight into the Sentinel process itself.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Live Updates</h3>
              <p>
                Metrics are pushed from the server every 2 seconds over
                WebSocket. No manual refresh is needed while connected.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Data Source</h3>
              <p>
                All metrics are collected locally by the Sentinel backend. No
                external monitoring agents or services are required.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
