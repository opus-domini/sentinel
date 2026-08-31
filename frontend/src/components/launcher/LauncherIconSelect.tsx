import { useMemo } from 'react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DEFAULT_ICON_KEY, TMUX_ICONS, TmuxIcon } from '@/lib/tmuxIcons'

type LauncherIconSelectProps = {
  labelId: string
  value: string
  onChange: (iconKey: string) => void
}

/** Labelled icon picker shared by the window and session launcher dialogs. */
export default function LauncherIconSelect({ labelId, value, onChange }: LauncherIconSelectProps) {
  const selectedEntry = useMemo(
    () =>
      TMUX_ICONS.find((entry) => entry.key === value) ??
      TMUX_ICONS.find((entry) => entry.key === DEFAULT_ICON_KEY) ??
      TMUX_ICONS[0],
    [value],
  )

  return (
    <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
      <span id={labelId}>Icon</span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            aria-labelledby={labelId}
            className="w-full cursor-pointer justify-start bg-surface-overlay text-[12px]"
          >
            <TmuxIcon iconKey={selectedEntry.key} className="h-3.5 w-3.5 text-muted-foreground" />
            {selectedEntry.label}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="z-[60]">
          {TMUX_ICONS.map((entry) => {
            const Icon = entry.icon
            return (
              <DropdownMenuItem
                key={entry.key}
                className="cursor-pointer"
                onSelect={() => onChange(entry.key)}
              >
                <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                {entry.label}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
