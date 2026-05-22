import { ChevronDown, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import CreateSessionDialog from './CreateSessionDialog'
import SessionLaunchersDialog from './SessionLaunchersDialog'
import SidebarHeader from './SidebarHeader'
import TokenDialog from './TokenDialog'
import type { SessionLauncher } from '@/types'
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
  launchers: Array<SessionLauncher>
  tmuxUnavailable: boolean
  filter: string
  onFilterChange: (value: string) => void
  onTokenChange: (value: string) => void
  onCreate: (name: string, cwd: string, user?: string) => void
  onLaunchLauncher: (id: string) => void
  onSaveLauncher: (input: {
    id: string
    name: string
    cwd: string
    icon: string
    user: string
  }) => Promise<string>
  onDeleteLauncher: (id: string) => Promise<boolean>
  onReorderLaunchers: (activeID: string, overID: string) => void
}

function describeSessionLauncher(launcher: SessionLauncher) {
  const user = launcher.user?.trim() ?? ''
  if (user === '') {
    return launcher.cwd
  }
  return `${launcher.cwd} · ${user}`
}

export default function SessionControls({
  sessionCount,
  tokenRequired,
  authenticated,
  defaultCwd,
  launchers,
  tmuxUnavailable,
  filter,
  onFilterChange,
  onTokenChange,
  onCreate,
  onLaunchLauncher,
  onSaveLauncher,
  onDeleteLauncher,
  onReorderLaunchers,
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

  const recentLauncher = useMemo(() => {
    const usedLaunchers = launchers.filter((launcher) =>
      Number.isFinite(Date.parse(launcher.lastUsedAt)),
    )
    if (usedLaunchers.length === 0) {
      return null
    }
    return [...usedLaunchers].sort((left, right) => {
      const leftTime = left.lastUsedAt ? Date.parse(left.lastUsedAt) : 0
      const rightTime = right.lastUsedAt ? Date.parse(right.lastUsedAt) : 0
      if (leftTime !== rightTime) {
        return rightTime - leftTime
      }
      return (left.sortOrder ?? 0) - (right.sortOrder ?? 0)
    })[0]
  }, [launchers])

  const secondaryLaunchers = useMemo(
    () =>
      recentLauncher === null
        ? launchers
        : launchers.filter((launcher) => launcher.id !== recentLauncher.id),
    [launchers, recentLauncher],
  )

  const addControl = (
    <div className="flex items-center text-foreground">
      <TooltipHelper content={tmuxUnavailable ? 'tmux not available' : 'New session'}>
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
          {recentLauncher !== null && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>Last used</DropdownMenuLabel>
              <DropdownMenuItem onSelect={() => onLaunchLauncher(recentLauncher.id)}>
                {(() => {
                  const Icon = getTmuxIcon(recentLauncher.icon)
                  return <Icon className="h-3.5 w-3.5" />
                })()}
                <span className="flex min-w-0 flex-1 items-center gap-2">
                  <span className="truncate">{recentLauncher.name}</span>
                  <span className="truncate text-[10px] text-muted-foreground">
                    {describeSessionLauncher(recentLauncher)}
                  </span>
                </span>
              </DropdownMenuItem>
            </>
          )}
          {secondaryLaunchers.length > 0 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>Session launchers</DropdownMenuLabel>
              {secondaryLaunchers.map((launcher) => {
                const Icon = getTmuxIcon(launcher.icon)
                return (
                  <DropdownMenuItem
                    key={launcher.id}
                    onSelect={() => onLaunchLauncher(launcher.id)}
                  >
                    <Icon className="h-3.5 w-3.5" />
                    <span className="flex min-w-0 flex-1 items-center gap-2">
                      <span className="truncate">{launcher.name}</span>
                      <span className="truncate text-[10px] text-muted-foreground">
                        {describeSessionLauncher(launcher)}
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
          <p className="font-semibold uppercase tracking-[0.06em]">tmux not available</p>
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
        launchers={launchers}
        onSave={onSaveLauncher}
        onDelete={onDeleteLauncher}
        onReorder={onReorderLaunchers}
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
