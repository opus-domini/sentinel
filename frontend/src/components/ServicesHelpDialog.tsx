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

export default function ServicesHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Services">
        <Button
          variant="outline"
          size="icon-xs"
          className="cursor-pointer text-secondary-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Services"
        >
          <CircleHelp className="h-3 w-3" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Services</DialogTitle>
            <DialogDescription>How the Sentinel service management works.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">Tracked Services</h3>
              <p>
                Services pinned from the Browse panel can be isolated with the Pinned filter. Their
                active state is polled automatically.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Browse</h3>
              <p>
                The Browse panel discovers systemd (Linux) or launchd (macOS) units available on the
                host. The default view stays focused on service units, and the type filter lets you
                expand into timers, targets, sockets, and other unit kinds when needed. Pin any unit
                to add it to the tracked set.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Actions</h3>
              <p>
                Start, stop, and restart services directly from the detail view through the ops
                control plane.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Scopes</h3>
              <p>
                Services are categorized as <strong className="text-foreground">system</strong>{' '}
                (managed by root / the system daemon) or{' '}
                <strong className="text-foreground">user</strong> (managed by the current user
                session).
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Built-in</h3>
              <p>
                The <strong className="text-foreground">sentinel</strong> and{' '}
                <strong className="text-foreground">sentinel-updater</strong> services are built-in
                and always tracked.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
