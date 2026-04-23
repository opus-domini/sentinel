import { ChevronDown, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import CreateSessionDialog from './CreateSessionDialog'
import SessionLaunchersDialog from './SessionLaunchersDialog'
import SidebarHeader from './SidebarHeader'
import TokenDialog from './TokenDialog'
import type { SessionPreset } from '@/types'
import TmuxHelpDialog from '@/components/TmuxHelpDialog'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { getTmuxIcon } from '@/lib/tmuxIcons'

type SessionControlsProps = {
  sessionCount: number
  tokenRequired: boolean
  authenticated: boolean
  defaultCwd: string
  presets: Array<SessionPreset>
  tmuxUnavailable: boolean
  filter: string
  onFilterChange: (value: string) => void
  onTokenChange: (value: string) => void
  onCreate: (name: string, cwd: string, user?: string) => void
  onLaunchPreset: (name: string) => void
  onSavePreset: (input: {
    previousName: string
    name: string
    cwd: string
    icon: string
    user: string
  }) => Promise<boolean>
  onDeletePreset: (name: string) => Promise<boolean>
  onReorderPresets: (activeName: string, overName: string) => void
}

function describeSessionLauncher(preset: SessionPreset) {
  const user = preset.user?.trim() ?? ''
  if (user === '') {
    return preset.cwd
  }
  return `${preset.cwd} · ${user}`
}

export default function SessionControls({
  sessionCount,
  tokenRequired,
  authenticated,
  defaultCwd,
  presets,
  tmuxUnavailable,
  filter,
  onFilterChange,
  onTokenChange,
  onCreate,
  onLaunchPreset,
  onSavePreset,
  onDeletePreset,
  onReorderPresets,
}: SessionControlsProps) {
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [isLaunchersOpen, setIsLaunchersOpen] = useState(false)
  const [isTokenOpen, setIsTokenOpen] = useState(false)

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

  const recentPreset = useMemo(() => {
    const launchedPresets = presets.filter((preset) =>
      Number.isFinite(Date.parse(preset.lastLaunchedAt)),
    )
    if (launchedPresets.length === 0) {
      return null
    }
    return [...launchedPresets].sort((left, right) => {
      const leftTime = left.lastLaunchedAt ? Date.parse(left.lastLaunchedAt) : 0
      const rightTime = right.lastLaunchedAt
        ? Date.parse(right.lastLaunchedAt)
        : 0
      if (leftTime !== rightTime) {
        return rightTime - leftTime
      }
      return (left.sortOrder ?? 0) - (right.sortOrder ?? 0)
    })[0]
  }, [presets])

  const secondaryPresets = useMemo(
    () =>
      recentPreset === null
        ? presets
        : presets.filter((preset) => preset.name !== recentPreset.name),
    [presets, recentPreset],
  )

  const addControl = (
    <div className="flex items-center text-foreground">
      <TooltipHelper
        content={tmuxUnavailable ? 'tmux not available' : 'New session'}
      >
        <Button
          variant="outline"
          size="icon-xs"
          className="rounded-r-none border-r-0 text-foreground"
          onClick={() => setIsCreateOpen(true)}
          aria-label="New session"
          disabled={tmuxUnavailable}
        >
          <Plus className="h-3 w-3" />
        </Button>
      </TooltipHelper>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="icon-xs"
            className="rounded-l-none px-0 text-foreground"
            aria-label="Open session launcher menu"
            disabled={tmuxUnavailable}
          >
            <ChevronDown className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem onSelect={() => setIsCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5" />
            New blank session
          </DropdownMenuItem>
          {recentPreset !== null && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>Last used</DropdownMenuLabel>
              <DropdownMenuItem
                onSelect={() => onLaunchPreset(recentPreset.name)}
              >
                {(() => {
                  const Icon = getTmuxIcon(recentPreset.icon)
                  return <Icon className="h-3.5 w-3.5" />
                })()}
                <span className="flex min-w-0 flex-1 items-center gap-2">
                  <span className="truncate">{recentPreset.name}</span>
                  <span className="truncate text-[10px] text-muted-foreground">
                    {describeSessionLauncher(recentPreset)}
                  </span>
                </span>
              </DropdownMenuItem>
            </>
          )}
          {secondaryPresets.length > 0 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>Session launchers</DropdownMenuLabel>
              {secondaryPresets.map((preset) => {
                const Icon = getTmuxIcon(preset.icon)
                return (
                  <DropdownMenuItem
                    key={preset.name}
                    onSelect={() => onLaunchPreset(preset.name)}
                  >
                    <Icon className="h-3.5 w-3.5" />
                    <span className="flex min-w-0 flex-1 items-center gap-2">
                      <span className="truncate">{preset.name}</span>
                      <span className="truncate text-[10px] text-muted-foreground">
                        {describeSessionLauncher(preset)}
                      </span>
                    </span>
                  </DropdownMenuItem>
                )
              })}
            </>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setIsLaunchersOpen(true)}>
            Manage session launchers...
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )

  return (
    <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
      <SidebarHeader
        title="Sessions"
        count={sessionCount}
        hasToken={authenticated}
        lockTitle={lockLabel}
        canCreate={!tmuxUnavailable}
        helpDialog={
          <TmuxHelpDialog
            buttonVariant="outline"
            buttonSize="icon-xs"
            buttonClassName="cursor-pointer text-secondary-foreground"
            iconClassName="h-3 w-3"
          />
        }
        addControl={addControl}
        onToggleAdd={() => setIsCreateOpen(true)}
        onToggleLock={() => setIsTokenOpen(true)}
      />

      {tmuxUnavailable && (
        <div className="rounded-md border border-warning/45 bg-warning/20 px-2.5 py-2 text-[11px] text-warning-foreground">
          <p className="font-semibold uppercase tracking-[0.06em]">
            tmux not available
          </p>
          <p className="mt-1 text-secondary-foreground">
            Install tmux on this host and restart Sentinel.
          </p>
        </div>
      )}

      <Input
        className="bg-surface-overlay"
        name="sessions-filter"
        placeholder="filter sessions..."
        value={filter}
        onChange={(event) => onFilterChange(event.target.value)}
      />

      <CreateSessionDialog
        open={isCreateOpen}
        onOpenChange={setIsCreateOpen}
        defaultCwd={defaultCwd}
        onCreate={onCreate}
      />

      <SessionLaunchersDialog
        open={isLaunchersOpen}
        onOpenChange={setIsLaunchersOpen}
        defaultCwd={defaultCwd}
        presets={presets}
        onSave={onSavePreset}
        onDelete={onDeletePreset}
        onReorder={onReorderPresets}
      />

      <TokenDialog
        open={isTokenOpen}
        onOpenChange={setIsTokenOpen}
        authenticated={authenticated}
        onTokenChange={onTokenChange}
        tokenRequired={tokenRequired}
      />
    </section>
  )
}
