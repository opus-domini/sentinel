import { useCallback, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, Trash2 } from 'lucide-react'
import type { OpsRunbookRun } from '@/types'
import { RunbookExecutionReceipt } from '@/components/runbooks/RunbookExecutionReceipt'
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
  focusJobId?: string
  onFocusJob?: (jobId: string | null) => void
}

type JobFilter = 'all' | 'active' | 'approval' | 'failed' | 'succeeded'

export function RunbookJobHistory({
  jobs,
  onDeleteJob,
  onApproveJob,
  onRejectJob,
  focusJobId,
  onFocusJob,
}: RunbookJobHistoryProps) {
  const { formatDateTime } = useDateFormat()
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null)
  const [filter, setFilter] = useState<JobFilter>('all')
  const [actingJobId, setActingJobId] = useState<string | null>(null)
  const [appliedFocusJobId, setAppliedFocusJobId] = useState<string | null>(null)
  const focusTarget = focusJobId && jobs.some((job) => job.id === focusJobId) ? focusJobId : null

  if (focusTarget !== appliedFocusJobId) {
    setAppliedFocusJobId(focusTarget)
    if (focusTarget !== null) {
      setFilter('all')
      setExpandedJobId(focusTarget)
    }
  }

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

  const toggleJobExpand = useCallback(
    (jobId: string) => {
      const next = expandedJobId === jobId ? null : jobId
      setExpandedJobId(next)
      onFocusJob?.(next)
    },
    [expandedJobId, onFocusJob],
  )

  const deleteJob = useCallback(
    async (jobId: string) => {
      await onDeleteJob(jobId)
      if (expandedJobId === jobId) {
        setExpandedJobId(null)
        onFocusJob?.(null)
      }
    },
    [expandedJobId, onDeleteJob, onFocusJob],
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
            const isActive = isActiveRunbookJob(job)
            const isWaitingApproval = isWaitingApprovalRunbookJob(job)
            const isActing = actingJobId === job.id
            const progress = runbookJobProgress(job)
            const duration = formatRunbookDuration(runbookJobDurationMs(job))
            const remainingSteps = job.definition?.steps.slice(job.completedSteps) ?? []
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
                    {(job.source || job.targetName) && (
                      <p className="truncate text-[10px] text-muted-foreground">
                        {job.source && `Source: ${job.source}`}
                        {job.source && job.targetName && ' · '}
                        {job.targetName && `Service: ${job.targetName}`}
                      </p>
                    )}
                    {isWaitingApproval && (
                      <div className="mt-1 grid gap-1 rounded border border-warning/30 bg-warning/6 p-2">
                        <p className="text-[10px] font-medium text-warning-foreground">
                          Review the frozen execution context before deciding.
                        </p>
                        {(job.targetName || job.definition?.targetName) && (
                          <p className="text-[9px] text-muted-foreground">
                            Target:{' '}
                            <span className="font-mono text-foreground">
                              {job.targetName || job.definition?.targetName}
                            </span>
                          </p>
                        )}
                        {job.definition ? (
                          <div className="grid gap-0.5">
                            <p className="text-[9px] text-muted-foreground">
                              Remaining after approval:
                            </p>
                            {remainingSteps.length > 0 ? (
                              remainingSteps.map((step, index) => (
                                <p
                                  key={`${job.completedSteps + index}:${step.type}:${step.title}`}
                                  className="truncate text-[9px] text-foreground"
                                >
                                  {job.completedSteps + index + 1}. {step.type} · {step.title}
                                </p>
                              ))
                            ) : (
                              <p className="text-[9px] text-muted-foreground">
                                No additional steps.
                              </p>
                            )}
                          </div>
                        ) : (
                          <p className="text-[9px] text-warning-foreground">
                            Frozen steps are unavailable for this legacy execution.
                          </p>
                        )}
                      </div>
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
                {isExpanded && <RunbookExecutionReceipt job={job} />}
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
