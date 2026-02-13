import { useMemo, useState } from 'react'
import CreateSessionDialog from './CreateSessionDialog'
import SidebarHeader from './SidebarHeader'
import TokenDialog from './TokenDialog'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

type SessionControlsProps = {
  sessionCount: number
  tokenRequired: boolean
  defaultCwd: string
  tmuxUnavailable: boolean
  recoveryKilledCount: number
  filter: string
  token: string
  onFilterChange: (value: string) => void
  onTokenChange: (value: string) => void
  onCreate: (name: string, cwd: string) => void
  onOpenRecovery: () => void
}

export default function SessionControls({
  sessionCount,
  tokenRequired,
  defaultCwd,
  tmuxUnavailable,
  recoveryKilledCount,
  filter,
  token,
  onFilterChange,
  onTokenChange,
  onCreate,
  onOpenRecovery,
}: SessionControlsProps) {
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const hasToken = token.trim() !== ''

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return hasToken ? 'Token configured (required)' : 'Token required'
    }
    return hasToken ? 'Token configured' : 'No token'
  }, [hasToken, tokenRequired])

  return (
    <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
      <SidebarHeader
        title="Sessions"
        count={sessionCount}
        hasToken={hasToken}
        lockTitle={lockLabel}
        canCreate={!tmuxUnavailable}
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

      {recoveryKilledCount > 0 && (
        <div className="rounded-md border border-destructive/45 bg-destructive/10 px-2.5 py-2 text-[11px]">
          <p className="font-semibold uppercase tracking-[0.06em] text-destructive-foreground">
            Recovery available
          </p>
          <p className="mt-1 text-secondary-foreground">
            {recoveryKilledCount} session
            {recoveryKilledCount > 1 ? 's' : ''} interrupted.
          </p>
          <Button
            className="mt-2 h-7"
            variant="outline"
            type="button"
            onClick={onOpenRecovery}
          >
            Open Recovery Center
          </Button>
        </div>
      )}

      <Input
        className="bg-surface-overlay"
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

      <TokenDialog
        open={isTokenOpen}
        onOpenChange={setIsTokenOpen}
        token={token}
        onTokenChange={onTokenChange}
        tokenRequired={tokenRequired}
      />
    </section>
  )
}
