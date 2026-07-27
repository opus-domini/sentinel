import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Pin, ScrollText, SquareTerminal } from 'lucide-react'
import type { NowInProgressRun, NowInProgressSession } from '@/types'
import { useDateFormat } from '@/hooks/useDateFormat'
import { nowRunbookSearch, nowTmuxSearch } from '@/lib/nowPresentation'

type NowInProgressProps = {
  runs: Array<NowInProgressRun>
  sessions: Array<NowInProgressSession>
}

export function NowInProgress({ runs, sessions }: NowInProgressProps) {
  const { formatRelativeTime } = useDateFormat()
  const empty = runs.length === 0 && sessions.length === 0

  return (
    <section
      aria-labelledby="now-progress-title"
      className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-xl border border-border-subtle bg-surface-raised"
    >
      <header className="flex items-end justify-between gap-3 border-b border-border-subtle px-3 py-2.5 sm:px-4">
        <div>
          <p className="text-[9px] uppercase tracking-[0.13em] text-primary-text">Live context</p>
          <h2 id="now-progress-title" className="mt-0.5 text-[14px] font-semibold">
            In progress
          </h2>
        </div>
        <span className="text-[10px] text-muted-foreground">
          {runs.length + sessions.length} active
        </span>
      </header>

      <div className="min-h-0 overflow-y-auto p-2">
        {empty ? (
          <div className="grid min-h-36 place-items-center rounded-lg border border-dashed border-border-subtle bg-surface-sunken px-5 text-center">
            <div>
              <p className="text-[12px] font-medium text-foreground">Nothing is in flight</p>
              <p className="mt-1 text-[10px] leading-relaxed text-muted-foreground">
                Queued procedures and relevant Tmux sessions will appear here.
              </p>
            </div>
          </div>
        ) : (
          <div className="grid gap-3">
            {runs.length > 0 && (
              <div className="grid gap-1.5">
                <div className="flex items-center justify-between px-1">
                  <span className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
                    Procedures
                  </span>
                  <Link to="/runbooks" className="text-[9px] text-primary-text hover:underline">
                    Open Runbooks
                  </Link>
                </div>
                {runs.map((run) => {
                  const progress =
                    run.totalSteps > 0
                      ? Math.min(100, Math.round((run.completedSteps / run.totalSteps) * 100))
                      : 0
                  return (
                    <Link
                      key={run.id}
                      to="/runbooks"
                      search={nowRunbookSearch(run.runbookId, run.id)}
                      className="group rounded-lg border border-border-subtle bg-surface-elevated px-3 py-2.5 no-underline transition-colors hover:bg-surface-hover"
                    >
                      <div className="flex items-start gap-2">
                        <ScrollText className="mt-0.5 size-3.5 shrink-0 text-warning-foreground" />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center justify-between gap-2">
                            <p className="truncate text-[11px] font-semibold text-foreground">
                              {run.runbookName}
                            </p>
                            <ArrowUpRight className="size-3 shrink-0 text-muted-foreground group-hover:text-foreground" />
                          </div>
                          <p className="truncate text-[9px] text-muted-foreground">
                            {run.status} · {formatRelativeTime(run.createdAt)}
                            {run.currentStep ? ` · ${run.currentStep}` : ''}
                          </p>
                          <div className="mt-2 h-1 overflow-hidden rounded-full bg-surface-overlay">
                            <span
                              className="block h-full rounded-full bg-warning transition-[width]"
                              style={{ width: `${progress}%` }}
                            />
                          </div>
                        </div>
                      </div>
                    </Link>
                  )
                })}
              </div>
            )}

            {sessions.length > 0 && (
              <div className="grid gap-1.5">
                <div className="flex items-center justify-between px-1">
                  <span className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
                    Sessions
                  </span>
                  <Link to="/tmux" className="text-[9px] text-primary-text hover:underline">
                    Open Tmux
                  </Link>
                </div>
                {sessions.map((session) => (
                  <Link
                    key={`${session.user ?? ''}:${session.name}`}
                    to="/tmux"
                    search={nowTmuxSearch(session.name)}
                    className="group flex items-start gap-2 rounded-lg border border-border-subtle bg-surface-elevated px-3 py-2.5 no-underline transition-colors hover:bg-surface-hover"
                  >
                    <SquareTerminal className="mt-0.5 size-3.5 shrink-0 text-primary" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <p className="truncate text-[11px] font-semibold text-foreground">
                          {session.name}
                        </p>
                        {session.pinned && <Pin className="size-2.5 shrink-0 text-primary-text" />}
                      </div>
                      <p className="truncate text-[9px] text-muted-foreground">
                        {session.user ? `${session.user} · ` : ''}
                        {session.unreadPanes} unread panes · {session.unreadWindows} windows
                      </p>
                      <p className="text-[9px] text-muted-foreground">
                        active {formatRelativeTime(session.activityAt)}
                      </p>
                    </div>
                    <ArrowUpRight className="mt-0.5 size-3 shrink-0 text-muted-foreground group-hover:text-foreground" />
                  </Link>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </section>
  )
}
