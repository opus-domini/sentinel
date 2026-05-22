import { useCallback, useMemo, useState } from 'react'
import { CheckCircle2, ChevronDown, ChevronRight, Trash2, XCircle } from 'lucide-react'
import type { OpsRunbookRun } from '@/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useDateFormat } from '@/hooks/useDateFormat'
import {
  formatRunbookDuration,
  isActiveRunbookJob,
  isWaitingApprovalRunbookJob,
  runbookJobDurationMs,
  runbookJobProgress,
} from '@/lib/runbookPresentation'
import { cn } from '@/lib/utils'

function runbookJobStatusClass(status: string): string {
  const s = status.trim().toLowerCase()
  if (s === 'succeeded') return 'text-ok-foreground'
  if (s === 'failed') return 'text-destructive-foreground'
  if (s === 'running' || s === 'queued' || s === 'waiting_approval') {
    return 'text-warning-foreground'
  }
  return 'text-muted-foreground'
}

function runbookJobStatusLabel(status: string): string {
  const s = status.trim().toLowerCase()
  if (s === 'waiting_approval') return 'Waiting approval'
  return status
}

type RunbookJobHistoryProps = {
  jobs: Array<OpsRunbookRun>
  onDeleteJob: (jobId: string) => Promise<void>
  onApproveJob: (jobId: string) => Promise<void>
  onRejectJob: (jobId: string) => Promise<void>
}

type JobFilter = 'all' | 'active' | 'approval' | 'failed' | 'succeeded'

export function RunbookJobHistory({
  jobs,
  onDeleteJob,
  onApproveJob,
  onRejectJob,
}: RunbookJobHistoryProps) {
  const { formatDateTime } = useDateFormat()
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null)
  const [expandedStepIndices, setExpandedStepIndices] = useState<Set<number>>(new Set())
  const [filter, setFilter] = useState<JobFilter>('all')
  const [actingJobId, setActingJobId] = useState<string | null>(null)

  const filteredJobs = useMemo(() => {
    if (filter === 'all') return jobs
    return jobs.filter((job) => {
      const status = job.status.trim().toLowerCase()
      if (filter === 'active') return isActiveRunbookJob(job)
      if (filter === 'approval') return isWaitingApprovalRunbookJob(job)
      return status === filter
    })
  }, [filter, jobs])

  const counts = useMemo(
    () => ({
      active: jobs.filter(isActiveRunbookJob).length,
      approval: jobs.filter(isWaitingApprovalRunbookJob).length,
      failed: jobs.filter((job) => job.status.trim().toLowerCase() === 'failed').length,
      succeeded: jobs.filter((job) => job.status.trim().toLowerCase() === 'succeeded').length,
    }),
    [jobs],
  )

  const toggleJobExpand = useCallback((jobId: string) => {
    setExpandedJobId((prev) => (prev === jobId ? null : jobId))
    setExpandedStepIndices(new Set())
  }, [])

  const toggleStepExpand = useCallback((index: number) => {
    setExpandedStepIndices((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }, [])

  const deleteJob = useCallback(
    async (jobId: string) => {
      await onDeleteJob(jobId)
      if (expandedJobId === jobId) setExpandedJobId(null)
    },
    [expandedJobId, onDeleteJob],
  )

  const approveJob = useCallback(
    async (jobId: string) => {
      setActingJobId(jobId)
      try {
        await onApproveJob(jobId)
      } finally {
        setActingJobId(null)
      }
    },
    [onApproveJob],
  )

  const rejectJob = useCallback(
    async (jobId: string) => {
      setActingJobId(jobId)
      try {
        await onRejectJob(jobId)
      } finally {
        setActingJobId(null)
      }
    },
    [onRejectJob],
  )

  return (
    <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
      <ScrollArea className="h-full min-h-0">
        <div className="grid gap-1 p-2">
          <div className="grid gap-1 px-1 pt-1">
            <div className="flex items-center justify-between">
              <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                Job History
              </span>
              <span className="text-[10px] text-muted-foreground">
                {filteredJobs.length}/{jobs.length} runs
              </span>
            </div>
            <div className="flex flex-wrap gap-1">
              {[
                ['all', `All ${jobs.length}`],
                ['active', `Active ${counts.active}`],
                ['approval', `Approvals ${counts.approval}`],
                ['failed', `Failed ${counts.failed}`],
                ['succeeded', `Succeeded ${counts.succeeded}`],
              ].map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  className={cn(
                    'h-6 rounded border px-2 text-[10px] transition-colors',
                    filter === value
                      ? 'border-primary/40 bg-primary/10 text-primary-text'
                      : 'border-border-subtle text-muted-foreground hover:bg-surface-overlay',
                  )}
                  aria-pressed={filter === value}
                  onClick={() => setFilter(value as JobFilter)}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          {filteredJobs.map((job) => {
            const isExpanded = expandedJobId === job.id
            const steps = job.stepResults
            const isActive = isActiveRunbookJob(job)
            const isWaitingApproval = isWaitingApprovalRunbookJob(job)
            const isActing = actingJobId === job.id
            const progress = runbookJobProgress(job)
            const duration = formatRunbookDuration(runbookJobDurationMs(job))
            return (
              <div
                key={job.id}
                className="group/job overflow-hidden rounded border border-border-subtle bg-surface-elevated"
              >
                <div className="flex items-start gap-1.5 px-2.5 py-2">
                  <button
                    type="button"
                    className="mt-0.5 shrink-0 cursor-pointer text-muted-foreground"
                    onClick={() => toggleJobExpand(job.id)}
                    aria-label="Toggle job details"
                    aria-expanded={isExpanded}
                  >
                    {isExpanded ? (
                      <ChevronDown className="h-3 w-3" />
                    ) : (
                      <ChevronRight className="h-3 w-3" />
                    )}
                  </button>
                  <button
                    type="button"
                    className="min-w-0 flex-1 cursor-pointer text-left"
                    onClick={() => toggleJobExpand(job.id)}
                    aria-label="Toggle job details"
                    aria-expanded={isExpanded}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span
                        className={cn(
                          'text-[12px] font-semibold',
                          runbookJobStatusClass(job.status),
                        )}
                      >
                        {runbookJobStatusLabel(job.status)}
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        {job.completedSteps}/{job.totalSteps} steps
                      </span>
                    </div>
                    {isActive && (
                      <div className="mt-1 h-1 overflow-hidden rounded-full bg-surface-overlay">
                        <span
                          className="block h-full rounded-full bg-warning"
                          style={{ width: `${progress}%` }}
                        />
                      </div>
                    )}
                    <p className="truncate text-[10px] text-muted-foreground">
                      {formatDateTime(job.createdAt)}
                      {` · ${duration}`}
                      {job.currentStep && ` · ${job.currentStep}`}
                    </p>
                    {isWaitingApproval && (
                      <p className="mt-1 text-[10px] text-warning-foreground">
                        Review the recorded output and choose whether this run can continue.
                      </p>
                    )}
                    {job.error && (
                      <p className="mt-1 text-[10px] text-destructive-foreground">{job.error}</p>
                    )}
                    {job.parametersUsed && Object.keys(job.parametersUsed).length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {Object.entries(job.parametersUsed).map(([key, val]) => (
                          <Badge
                            key={key}
                            variant="outline"
                            className="h-4 gap-0.5 px-1 text-[9px]"
                          >
                            <span className="font-mono">{key}</span>
                            <span className="text-muted-foreground">=</span>
                            <span>{val}</span>
                          </Badge>
                        ))}
                      </div>
                    )}
                  </button>
                  {isWaitingApproval && (
                    <div className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        className="h-6 cursor-pointer rounded border border-ok/40 bg-ok/10 px-2 text-[10px] font-medium text-ok-foreground hover:bg-ok/20 disabled:cursor-not-allowed disabled:opacity-60"
                        disabled={isActing}
                        onClick={() => void approveJob(job.id)}
                        aria-label="Approve run"
                      >
                        Approve
                      </button>
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <button
                            type="button"
                            className="h-6 cursor-pointer rounded border border-destructive/40 bg-destructive/10 px-2 text-[10px] font-medium text-destructive-foreground hover:bg-destructive/20 disabled:cursor-not-allowed disabled:opacity-60"
                            disabled={isActing}
                            aria-label="Reject approval"
                          >
                            Reject
                          </button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Reject approval?</AlertDialogTitle>
                            <AlertDialogDescription>
                              This marks the run as failed and it will not execute the remaining
                              steps.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => void rejectJob(job.id)}
                            >
                              Reject
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  )}
                  {!isActive && (
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <button
                          type="button"
                          className="mt-0.5 shrink-0 cursor-pointer text-muted-foreground opacity-0 transition-opacity hover:text-destructive-foreground group-hover/job:opacity-100"
                          aria-label="Delete job"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>Delete job?</AlertDialogTitle>
                          <AlertDialogDescription>
                            This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction
                            variant="destructive"
                            onClick={() => void deleteJob(job.id)}
                          >
                            Delete
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  )}
                </div>
                {isExpanded && steps.length > 0 && (
                  <div className="grid min-w-0 gap-0.5 border-t border-border-subtle px-2.5 py-2">
                    {steps.map((sr) => {
                      const stepOpen = expandedStepIndices.has(sr.stepIndex)
                      return (
                        <div
                          key={sr.stepIndex}
                          className="min-w-0 overflow-hidden rounded border border-border-subtle bg-surface-overlay"
                        >
                          <button
                            type="button"
                            className="flex w-full cursor-pointer items-center justify-between gap-2 px-2 py-1.5 text-[11px]"
                            onClick={() => toggleStepExpand(sr.stepIndex)}
                          >
                            <div className="flex min-w-0 items-center gap-1.5">
                              <span className="shrink-0 text-muted-foreground">
                                {stepOpen ? (
                                  <ChevronDown className="h-2.5 w-2.5" />
                                ) : (
                                  <ChevronRight className="h-2.5 w-2.5" />
                                )}
                              </span>
                              {sr.error ? (
                                <XCircle className="h-3 w-3 shrink-0 text-destructive-foreground" />
                              ) : (
                                <CheckCircle2 className="h-3 w-3 shrink-0 text-ok-foreground" />
                              )}
                              <span className="shrink-0 rounded border border-border-subtle px-1 text-[9px] uppercase text-muted-foreground">
                                {sr.type}
                              </span>
                              <span className="truncate font-medium">{sr.title}</span>
                            </div>
                            <span className="shrink-0 text-[10px] text-muted-foreground">
                              {sr.durationMs}ms
                            </span>
                          </button>
                          {stepOpen && (
                            <div className="border-t border-border-subtle px-2 py-1.5">
                              {sr.output ? (
                                <pre className="max-h-40 overflow-y-auto overscroll-contain whitespace-pre-wrap rounded bg-[var(--background)] px-2 py-1 font-mono text-[10px] text-secondary-foreground">
                                  {sr.output}
                                </pre>
                              ) : !sr.error ? (
                                <p className="px-1 text-[10px] italic text-muted-foreground">
                                  No output
                                </p>
                              ) : null}
                              {sr.error && (
                                <p className="mt-1 text-[10px] text-destructive-foreground">
                                  {sr.error}
                                </p>
                              )}
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}
                {isExpanded && steps.length === 0 && (
                  <div className="border-t border-border-subtle px-2.5 py-2">
                    <p className="text-[10px] text-muted-foreground">No step output recorded.</p>
                  </div>
                )}
              </div>
            )
          })}
          {filteredJobs.length === 0 && (
            <p className="p-2 text-[12px] text-muted-foreground">
              {jobs.length === 0 ? 'No runs yet.' : 'No runs match this filter.'}
            </p>
          )}
        </div>
      </ScrollArea>
    </div>
  )
}
