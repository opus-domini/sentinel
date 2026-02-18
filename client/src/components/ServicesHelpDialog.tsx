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
          variant="ghost"
          size="icon"
          className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Services"
        >
          <CircleHelp className="h-4 w-4" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Services</DialogTitle>
            <DialogDescription>
              How the Sentinel service management works.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                Tracked Services
              </h3>
              <p>
                Services pinned from the Browse panel appear in the sidebar for
                quick status monitoring. Their active state is polled
                automatically.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Browse</h3>
              <p>
                The Browse panel discovers systemd (Linux) or launchd (macOS)
                units available on the host. Pin any unit to track it in the
                sidebar.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Actions</h3>
              <p>
                Start, stop, and restart services directly from the detail
                view. Actions are executed through the ops control plane and
                recorded in the timeline.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Scopes</h3>
              <p>
                Services are categorized as{' '}
                <strong className="text-foreground">system</strong> (managed by
                root / the system daemon) or{' '}
                <strong className="text-foreground">user</strong> (managed by
                the current user session).
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Built-in</h3>
              <p>
                The <strong className="text-foreground">sentinel</strong> and{' '}
                <strong className="text-foreground">sentinel-updater</strong>{' '}
                services are built-in and always tracked. They cannot be
                removed from the sidebar.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
