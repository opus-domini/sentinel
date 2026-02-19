import type {
  RecoveryJob,
  RecoverySession,
  RecoverySnapshotView,
} from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { slugifyTmuxName } from '@/lib/tmuxName'

type RecoveryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  recoverySessions: Array<RecoverySession>
  recoveryJobs: Array<RecoveryJob>
  recoverySnapshots: Array<{
    id: number
    capturedAt: string
    windows: number
    panes: number
  }>
  selectedRecoverySession: string | null
  selectedSnapshotID: number | null
  selectedSnapshot: RecoverySnapshotView | null
  recoveryLoading: boolean
  recoveryBusy: boolean
  recoveryError: string
  restoreMode: 'safe' | 'confirm' | 'full'
  restoreConflictPolicy: 'rename' | 'replace' | 'skip'
  restoreTargetSession: string
  onRefresh: () => void
  onSelectSession: (session: string) => void
  onSelectSnapshot: (snapshotID: number) => void
  onRestoreModeChange: (mode: 'safe' | 'confirm' | 'full') => void
  onConflictPolicyChange: (policy: 'rename' | 'replace' | 'skip') => void
  onTargetSessionChange: (session: string) => void
  onRestore: () => void
  onArchive: (session: string) => void
}

export default function RecoveryDialog(props: RecoveryDialogProps) {
  const {
    open,
    onOpenChange,
    recoverySessions,
    recoveryJobs,
    recoverySnapshots,
    selectedRecoverySession,
    selectedSnapshotID,
    selectedSnapshot,
    recoveryLoading,
    recoveryBusy,
    recoveryError,
    restoreMode,
    restoreConflictPolicy,
    restoreTargetSession,
    onRefresh,
    onSelectSession,
    onSelectSnapshot,
    onRestoreModeChange,
    onConflictPolicyChange,
    onTargetSessionChange,
    onRestore,
    onArchive,
  } = props

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (nextOpen) onRefresh()
      }}
    >
      <DialogContent className="max-h-[88vh] overflow-hidden sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>Recovery Center</DialogTitle>
          <DialogDescription>
            Restore tmux sessions interrupted by reboot or power loss.
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 gap-3 md:grid-cols-[15rem_1fr]">
          <section className="grid min-h-0 gap-2 rounded-md border border-border-subtle bg-secondary p-2">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Sessions
              </span>
              <Button
                size="sm"
                variant="outline"
                className="h-6 px-2 text-[11px]"
                type="button"
                onClick={onRefresh}
                disabled={recoveryLoading}
              >
                Refresh
              </Button>
            </div>
            <div className="min-h-0 overflow-auto">
              {recoverySessions.length === 0 ? (
                <p className="px-1 py-2 text-[12px] text-muted-foreground">
                  No recoverable sessions.
                </p>
              ) : (
                <ul className="grid gap-1">
                  {recoverySessions.map((item) => (
                    <li key={item.name}>
                      <button
                        type="button"
                        className={`w-full rounded-md border px-2 py-1.5 text-left text-[12px] ${
                          selectedRecoverySession === item.name
                            ? 'border-primary/60 bg-primary/10'
                            : 'border-border-subtle bg-surface-overlay hover:border-border'
                        }`}
                        onClick={() => {
                          onSelectSession(item.name)
                        }}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="truncate font-medium">
                            {item.name}
                          </span>
                          <Badge
                            variant={
                              item.state === 'restored'
                                ? 'secondary'
                                : item.state === 'restoring'
                                  ? 'outline'
                                  : 'destructive'
                            }
                          >
                            {item.state}
                          </Badge>
                        </div>
                        <div className="mt-1 text-[10px] text-muted-foreground">
                          {item.windows} windows · {item.panes} panes
                        </div>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </section>

          <section className="grid min-h-0 grid-rows-[auto_auto_1fr_auto] gap-2 rounded-md border border-border-subtle bg-secondary p-3">
            <div className="flex items-center gap-2">
              <Badge variant="outline">
                {selectedRecoverySession ?? 'Select a session'}
              </Badge>
              {recoveryBusy && <Badge variant="outline">Restoring…</Badge>}
            </div>

            <div className="grid gap-2 md:grid-cols-3">
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">
                  Snapshot
                </span>
                <Select
                  value={selectedSnapshotID ? String(selectedSnapshotID) : ''}
                  onValueChange={(value) => {
                    const id = Number(value)
                    if (Number.isFinite(id) && id > 0) {
                      onSelectSnapshot(id)
                    }
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Choose snapshot" />
                  </SelectTrigger>
                  <SelectContent>
                    {recoverySnapshots.map((item) => (
                      <SelectItem key={item.id} value={String(item.id)}>
                        #{item.id} ·{' '}
                        {new Date(item.capturedAt).toLocaleString()}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">
                  Replay mode
                </span>
                <Select
                  value={restoreMode}
                  onValueChange={(value) =>
                    onRestoreModeChange(value as 'safe' | 'confirm' | 'full')
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="safe">safe</SelectItem>
                    <SelectItem value="confirm">confirm</SelectItem>
                    <SelectItem value="full">full</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">
                  Name conflict
                </span>
                <Select
                  value={restoreConflictPolicy}
                  onValueChange={(value) =>
                    onConflictPolicyChange(
                      value as 'rename' | 'replace' | 'skip',
                    )
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="rename">rename</SelectItem>
                    <SelectItem value="replace">replace</SelectItem>
                    <SelectItem value="skip">skip</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid min-h-0 gap-2 overflow-auto rounded-md border border-border-subtle bg-surface-overlay p-2">
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">
                  Target session
                </span>
                <Input
                  value={restoreTargetSession}
                  onChange={(event) =>
                    onTargetSessionChange(slugifyTmuxName(event.target.value))
                  }
                  placeholder="restored session name"
                />
              </div>
              {selectedSnapshot ? (
                <div className="grid gap-2 text-[12px]">
                  <div className="text-muted-foreground">
                    Captured:{' '}
                    {new Date(
                      selectedSnapshot.payload.capturedAt,
                    ).toLocaleString()}
                  </div>
                  <div className="text-muted-foreground">
                    {selectedSnapshot.payload.windows.length} windows ·{' '}
                    {selectedSnapshot.payload.panes.length} panes
                  </div>
                  <div className="grid gap-1">
                    <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                      Windows
                    </span>
                    <div className="max-h-24 overflow-auto rounded border border-border-subtle bg-secondary p-1 text-[11px]">
                      {selectedSnapshot.payload.windows.map((window) => (
                        <div key={`${window.index}-${window.name}`}>
                          #{window.index} {window.name} ({window.panes} panes)
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className="grid gap-1">
                    <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                      Panes
                    </span>
                    <div className="max-h-28 overflow-auto rounded border border-border-subtle bg-secondary p-1 text-[11px]">
                      {selectedSnapshot.payload.panes.map((pane) => (
                        <div
                          key={`${pane.windowIndex}-${pane.paneIndex}-${pane.title}`}
                        >
                          {pane.windowIndex}.{pane.paneIndex} ·{' '}
                          {pane.currentPath || '~'}
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              ) : (
                <p className="text-[12px] text-muted-foreground">
                  Select a snapshot to inspect and restore.
                </p>
              )}
            </div>

            <DialogFooter className="items-center justify-between">
              <div className="min-h-[1.25rem] text-[11px] text-destructive-foreground">
                {recoveryError}
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => {
                    if (selectedRecoverySession) {
                      onArchive(selectedRecoverySession)
                    }
                  }}
                  disabled={selectedRecoverySession === null || recoveryBusy}
                >
                  Archive
                </Button>
                <Button
                  type="button"
                  onClick={onRestore}
                  disabled={selectedSnapshotID === null || recoveryBusy}
                >
                  Restore Snapshot
                </Button>
              </div>
            </DialogFooter>
          </section>
        </div>

        {recoveryJobs.length > 0 && (
          <div className="mt-2 rounded-md border border-border-subtle bg-surface-overlay p-2 text-[11px]">
            <p className="font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Recent Jobs
            </p>
            <div className="mt-1 grid gap-1">
              {recoveryJobs.slice(0, 6).map((job) => (
                <div
                  key={job.id}
                  className="flex items-center justify-between gap-2"
                >
                  <span className="truncate">
                    {job.sessionName} → {job.targetSession || job.sessionName}
                  </span>
                  <span className="tabular-nums text-muted-foreground">
                    {job.completedSteps}/{job.totalSteps} · {job.status}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
