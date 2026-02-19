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

export default function RunbooksHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Runbooks">
        <Button
          variant="ghost"
          size="icon"
          className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Runbooks"
        >
          <CircleHelp className="h-4 w-4" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Runbooks</DialogTitle>
            <DialogDescription>
              How the Sentinel runbook system works.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                What are Runbooks
              </h3>
              <p>
                Runbooks are automated sequences of steps that execute commands
                on the host. They codify routine operational procedures into
                repeatable workflows.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Step Types</h3>
              <p>
                <strong className="text-foreground">command</strong> — runs a
                shell command and captures its output.
              </p>
              <p className="mt-1">
                <strong className="text-foreground">check</strong> — runs a
                validation command and fails the runbook if it exits non-zero.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Execution</h3>
              <p>
                Steps run sequentially. Each step&apos;s output is captured and
                displayed in real time. A failing step halts the remaining
                sequence.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Job History</h3>
              <p>
                Every run is recorded as a job with its status (succeeded,
                failed, running) and timestamps. Past results are available in
                the runbook detail view.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
