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

export default function GuardrailsHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Guardrails">
        <Button
          variant="outline"
          size="sm"
          className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
          onClick={() => setOpen(true)}
          aria-label="About Guardrails"
        >
          <CircleHelp className="h-3.5 w-3.5" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Guardrails</DialogTitle>
            <DialogDescription>
              Safety rules that evaluate actions before execution.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                What are Guardrails?
              </h3>
              <p>
                Guardrails are safety rules that evaluate operations before
                execution. Each rule matches against one or more actions and
                determines how the system should respond.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Modes</h3>
              <ul className="list-inside list-disc space-y-1">
                <li>
                  <strong className="text-foreground">warn</strong> — log the
                  match and allow execution to proceed
                </li>
                <li>
                  <strong className="text-foreground">confirm</strong> — require
                  explicit confirmation before execution
                </li>
                <li>
                  <strong className="text-foreground">block</strong> — deny
                  execution entirely
                </li>
              </ul>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Priority</h3>
              <p>
                Lower number means higher priority. When multiple rules match
                the same action, the strictest mode wins.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Audit Log</h3>
              <p>
                Every guardrail evaluation is recorded in the audit log for
                review. Switch to the Audit Log tab to see past evaluations.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
