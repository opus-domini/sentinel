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

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded border border-border bg-surface-overlay px-1 font-mono text-[10px] text-foreground">
      {children}
    </kbd>
  )
}

type Shortcut = { keys: string; desc: string }

const tmuxShortcuts: ReadonlyArray<Shortcut> = [
  { keys: 'Ctrl-b [', desc: 'Copy mode (scroll history, q to exit)' },
  { keys: 'Ctrl-b c', desc: 'Create new window' },
  { keys: 'Ctrl-b n', desc: 'Next window' },
  { keys: 'Ctrl-b p', desc: 'Previous window' },
  { keys: 'Ctrl-b d', desc: 'Detach from session' },
  { keys: 'Ctrl-b %', desc: 'Split pane vertically' },
  { keys: 'Ctrl-b "', desc: 'Split pane horizontally' },
  { keys: 'Ctrl-b o', desc: 'Switch to next pane' },
  { keys: 'Ctrl-b x', desc: 'Close current pane' },
  { keys: 'Ctrl-b ,', desc: 'Rename current window' },
]

const shellShortcuts: ReadonlyArray<Shortcut> = [
  { keys: 'Ctrl-c', desc: 'Interrupt running command' },
  { keys: 'Ctrl-l', desc: 'Clear screen' },
  { keys: 'Ctrl-a', desc: 'Move cursor to start of line' },
  { keys: 'Ctrl-e', desc: 'Move cursor to end of line' },
]

export default function TmuxHelpDialog() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <TooltipHelper content="About Terminal">
        <Button
          variant="ghost"
          size="icon"
          className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
          onClick={() => setOpen(true)}
          aria-label="About Terminal"
        >
          <CircleHelp className="h-4 w-4" />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>About Terminal</DialogTitle>
            <DialogDescription>
              How the Sentinel terminal interface works.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <section>
              <h3 className="mb-1 font-medium text-foreground">Sessions</h3>
              <p>
                Each tab maps to a tmux session running on the host. Sessions
                persist across disconnects — closing a tab detaches without
                killing the session. Use the sidebar to create, attach, or kill
                sessions.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Terminal</h3>
              <p>
                The terminal is rendered with xterm.js and connected to the
                host via a WebSocket PTY. Input and output are streamed in
                real time.
              </p>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                Keyboard Shortcuts — tmux
              </h3>
              <div className="space-y-1.5">
                {tmuxShortcuts.map(({ keys, desc }) => (
                  <div key={keys} className="flex items-start gap-2 text-xs">
                    <Kbd>{keys}</Kbd>
                    <span className="pt-0.5 text-muted-foreground">{desc}</span>
                  </div>
                ))}
              </div>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">
                Keyboard Shortcuts — Shell
              </h3>
              <div className="space-y-1.5">
                {shellShortcuts.map(({ keys, desc }) => (
                  <div key={keys} className="flex items-start gap-2 text-xs">
                    <Kbd>{keys}</Kbd>
                    <span className="pt-0.5 text-muted-foreground">{desc}</span>
                  </div>
                ))}
              </div>
            </section>
            <section>
              <h3 className="mb-1 font-medium text-foreground">Recovery</h3>
              <p>
                Sentinel periodically snapshots terminal content. If a session
                is interrupted, the recovery center lets you restore from the
                last captured snapshot.
              </p>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
