import { useMemo, useState } from 'react'
import { Clock, Lock, LockOpen, Plus, Webhook } from 'lucide-react'
import type { OpsRunbook, OpsRunbookRun, OpsSchedule } from '@/types'
import RunbooksHelpDialog from '@/components/RunbooksHelpDialog'
import SidebarShell from '@/components/sidebar/SidebarShell'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type RunbooksSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  loading: boolean
  runbooks: Array<OpsRunbook>
  jobs: Array<OpsRunbookRun>
  schedules: Array<OpsSchedule>
  selectedRunbookId: string | null
  onTokenChange: (value: string) => void
  onSelectRunbook: (id: string | null) => void
  onCreateRunbook?: () => void
}

function runbookStatusDot(
  runbook: OpsRunbook,
  jobs: Array<OpsRunbookRun>,
): string {
  const lastJob = jobs.find((j) => j.runbookId === runbook.id)
  if (!lastJob) return 'bg-muted-foreground/50'
  const status = lastJob.status.trim().toLowerCase()
  if (status === 'succeeded') return 'bg-emerald-500'
  if (status === 'failed') return 'bg-red-500'
  if (status === 'running') return 'bg-amber-500'
  return 'bg-muted-foreground/50'
}

export default function RunbooksSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  authenticated,
  loading,
  runbooks,
  jobs,
  schedules,
  selectedRunbookId,
  onTokenChange,
  onSelectRunbook,
  onCreateRunbook,
}: RunbooksSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const [filter, setFilter] = useState('')

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

  const filteredRunbooks = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (q === '') return runbooks
    return runbooks.filter(
      (rb) =>
        rb.name.toLowerCase().includes(q) ||
        rb.description.toLowerCase().includes(q),
    )
  }, [runbooks, filter])

  const hasFilter = filter.trim() !== ''

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Runbooks
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {runbooks.length}
            </span>
            <div className="ml-auto flex items-center gap-1.5">
              <RunbooksHelpDialog />
              {onCreateRunbook && (
                <TooltipHelper content="New runbook">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
                    onClick={onCreateRunbook}
                    aria-label="New runbook"
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </TooltipHelper>
              )}
              <TooltipHelper content={lockLabel}>
                <Button
                  variant="ghost"
                  size="icon"
                  className="cursor-pointer border border-border bg-surface-hover text-secondary-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => setIsTokenOpen(true)}
                  aria-label="API token"
                >
                  {authenticated ? (
                    <Lock className="h-4 w-4" />
                  ) : (
                    <LockOpen className="h-4 w-4" />
                  )}
                </Button>
              </TooltipHelper>
            </div>
          </div>
          <Input
            className="bg-surface-overlay text-[12px] md:h-8"
            placeholder="filter runbooks..."
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />

          <TokenDialog
            open={isTokenOpen}
            onOpenChange={setIsTokenOpen}
            authenticated={authenticated}
            onTokenChange={onTokenChange}
            tokenRequired={tokenRequired}
          />
        </section>

        <section className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <div className="flex items-center justify-between border-b border-border-subtle px-2 py-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Available
            </span>
            <span className="text-[10px] text-muted-foreground">
              {loading
                ? 'syncing...'
                : hasFilter
                  ? `${filteredRunbooks.length}/${runbooks.length}`
                  : `${runbooks.length} runbooks`}
            </span>
          </div>
          <ScrollArea className="h-full min-h-0">
            <div className="grid min-h-0 gap-1.5 p-2">
              {loading && runbooks.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  Loading runbooks...
                </EmptyState>
              )}

              {!loading && filteredRunbooks.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="grid gap-1 border-dashed p-3 text-[12px]"
                >
                  <span>
                    {hasFilter
                      ? 'No runbooks match filter.'
                      : 'No runbooks available.'}
                  </span>
                  {hasFilter && (
                    <Button
                      variant="outline"
                      className="mx-auto h-7 px-2 text-[11px]"
                      type="button"
                      onClick={() => setFilter('')}
                    >
                      Clear Filter
                    </Button>
                  )}
                </EmptyState>
              )}

              {filteredRunbooks.map((runbook) => {
                const lastJob = jobs.find((j) => j.runbookId === runbook.id)
                const isSelected = selectedRunbookId === runbook.id
                return (
                  <button
                    key={runbook.id}
                    type="button"
                    className={cn(
                      'grid min-w-0 cursor-pointer gap-1 overflow-hidden rounded border px-2 py-1.5 text-left text-[12px] transition-colors',
                      isSelected
                        ? 'border-primary/40 bg-primary/10'
                        : 'border-border-subtle hover:border-border hover:bg-surface-overlay',
                    )}
                    onClick={() =>
                      onSelectRunbook(isSelected ? null : runbook.id)
                    }
                  >
                    <div className="flex min-w-0 items-center gap-1.5">
                      <span
                        className={cn(
                          'h-2 w-2 shrink-0 rounded-full',
                          runbookStatusDot(runbook, jobs),
                        )}
                      />
                      {schedules.some((s) => s.runbookId === runbook.id) && (
                        <Clock className="h-2.5 w-2.5 shrink-0 text-muted-foreground" />
                      )}
                      {runbook.webhookURL && (
                        <Webhook className="h-2.5 w-2.5 shrink-0 text-muted-foreground" />
                      )}
                      <span className="min-w-0 flex-1 truncate font-semibold">
                        {runbook.name}
                      </span>
                      <span className="shrink-0 text-[10px] text-muted-foreground">
                        {runbook.steps.length} steps
                      </span>
                    </div>
                    <span className="truncate text-[10px] text-muted-foreground">
                      {lastJob
                        ? `last: ${lastJob.status} · ${lastJob.createdAt}`
                        : 'never ran'}
                    </span>
                  </button>
                )
              })}
            </div>
          </ScrollArea>
        </section>
      </div>
    </SidebarShell>
  )
}
