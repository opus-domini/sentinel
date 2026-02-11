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

type ShortcutSection = { title: string; items: ReadonlyArray<Shortcut> }

const tmuxSections: ReadonlyArray<ShortcutSection> = [
  {
    title: 'tmux',
    items: [
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
    ],
  },
  {
    title: 'Shell',
    items: [
      { keys: 'Ctrl-c', desc: 'Interrupt running command' },
      { keys: 'Ctrl-l', desc: 'Clear screen' },
      { keys: 'Ctrl-a', desc: 'Move cursor to start of line' },
      { keys: 'Ctrl-e', desc: 'Move cursor to end of line' },
    ],
  },
]

const terminalSections: ReadonlyArray<ShortcutSection> = [
  {
    title: 'Terminal',
    items: [
      { keys: 'Scroll wheel', desc: 'Scroll terminal buffer' },
      { keys: 'Ctrl-c', desc: 'Interrupt running command' },
      { keys: 'Ctrl-l', desc: 'Clear screen' },
      { keys: 'Ctrl-a', desc: 'Move cursor to start of line' },
      { keys: 'Ctrl-e', desc: 'Move cursor to end of line' },
      { keys: 'Ctrl-d', desc: 'Exit shell / EOF' },
    ],
  },
]

const sectionsByContext = {
  tmux: tmuxSections,
  terminal: terminalSections,
} as const

const descriptionByContext = {
  tmux: 'Common tmux and shell shortcuts.',
  terminal: 'Common terminal shortcuts.',
} as const

type HelpDialogProps = {
  context: 'tmux' | 'terminal'
  triggerClassName?: string
  iconClassName?: string
}

export default function HelpDialog({
  context,
  triggerClassName,
  iconClassName,
}: HelpDialogProps) {
  const [open, setOpen] = useState(false)
  const sections = sectionsByContext[context]

  return (
    <>
      <TooltipHelper content="Keyboard shortcuts">
        <Button
          variant="ghost"
          size="icon-sm"
          className={triggerClassName}
          onClick={() => setOpen(true)}
          aria-label="Keyboard shortcuts"
        >
          <CircleHelp className={iconClassName ?? 'h-3.5 w-3.5'} />
        </Button>
      </TooltipHelper>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Keyboard Shortcuts</DialogTitle>
            <DialogDescription>
              {descriptionByContext[context]}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {sections.map((section) => (
              <section key={section.title}>
                <h3 className="mb-2 text-xs font-medium text-foreground">
                  {section.title}
                </h3>
                <div className="space-y-1.5">
                  {section.items.map(({ keys, desc }) => (
                    <div key={keys} className="flex items-start gap-2 text-xs">
                      <Kbd>{keys}</Kbd>
                      <span className="pt-0.5 text-muted-foreground">
                        {desc}
                      </span>
                    </div>
                  ))}
                </div>
              </section>
            ))}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
