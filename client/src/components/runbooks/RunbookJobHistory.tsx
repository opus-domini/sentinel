import { useCallback, useEffect, useState } from 'react'
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Trash2,
  XCircle,
} from 'lucide-react'
import type { OpsRunbookRun } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useDateFormat } from '@/hooks/useDateFormat'
import { cn } from '@/lib/utils'

function runbookJobStatusClass(status: string): string {
  const s = status.trim().toLowerCase()
  if (s === 'succeeded') return 'text-emerald-400'
  if (s === 'failed') return 'text-red-400'
  if (s === 'running') return 'text-amber-400'
  return 'text-muted-foreground'
}

type RunbookJobHistoryProps = {
  jobs: Array<OpsRunbookRun>
  onDeleteJob: (jobId: string) => Promise<void>
}

export function RunbookJobHistory({
  jobs,
  onDeleteJob,
}: RunbookJobHistoryProps) {
  const { formatDateTime } = useDateFormat()
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null)
  const [expandedStepIndices, setExpandedStepIndices] = useState<Set<number>>(
    new Set(),
  )
  const [deleteJobTarget, setDeleteJobTarget] = useState<string | null>(null)

  useEffect(() => {
    if (deleteJobTarget == null) return
    const timer = setTimeout(() => setDeleteJobTarget(null), 3000)
    return () => clearTimeout(timer)
  }, [deleteJobTarget])

  const toggleJobExpand = useCallback((jobId: string) => {
    setExpandedJobId((prev) => (prev === jobId ? null : jobId))
    setExpandedStepIndices(new Set())
    setDeleteJobTarget(null)
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
      setDeleteJobTarget(null)
    },
    [expandedJobId, onDeleteJob],
  )

  return (
    <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
      <ScrollArea className="h-full min-h-0">
        <div className="grid gap-1 p-2">
          <div className="flex items-center justify-between px-1 pt-1">
            <span className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
              Job History
            </span>
            <span className="text-[10px] text-muted-foreground">
              {jobs.length} runs
            </span>
          </div>
          {jobs.map((job) => {
            const isExpanded = expandedJobId === job.id
            const steps = job.stepResults
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
                  <div
                    className="min-w-0 flex-1 cursor-pointer"
                    role="button"
                    tabIndex={0}
                    onClick={() => toggleJobExpand(job.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ')
                        toggleJobExpand(job.id)
                    }}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span
                        className={cn(
                          'text-[12px] font-semibold',
                          runbookJobStatusClass(job.status),
                        )}
                      >
                        {job.status}
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        {job.completedSteps}/{job.totalSteps} steps
                      </span>
                    </div>
                    <p className="truncate text-[10px] text-muted-foreground">
                      {formatDateTime(job.createdAt)}
                      {job.currentStep && ` · ${job.currentStep}`}
                    </p>
                    {job.error && (
                      <p className="mt-1 text-[10px] text-red-400">
                        {job.error}
                      </p>
                    )}
                    {job.parametersUsed &&
                      Object.keys(job.parametersUsed).length > 0 && (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {Object.entries(job.parametersUsed).map(
                            ([key, val]) => (
                              <Badge
                                key={key}
                                variant="outline"
                                className="h-4 gap-0.5 px-1 text-[9px]"
                              >
                                <span className="font-mono">{key}</span>
                                <span className="text-muted-foreground">=</span>
                                <span>{val}</span>
                              </Badge>
                            ),
                          )}
                        </div>
                      )}
                  </div>
                  {job.status !== 'running' &&
                    (deleteJobTarget === job.id ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-5 shrink-0 cursor-pointer px-1.5 text-[10px] text-red-400 hover:text-red-300"
                        onClick={() => {
                          setDeleteJobTarget(null)
                          void deleteJob(job.id)
                        }}
                      >
                        Confirm?
                      </Button>
                    ) : (
                      <button
                        type="button"
                        className="mt-0.5 shrink-0 cursor-pointer text-muted-foreground opacity-0 transition-opacity hover:text-red-400 group-hover/job:opacity-100"
                        onClick={() => setDeleteJobTarget(job.id)}
                        aria-label="Delete job"
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    ))}
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
                                <XCircle className="h-3 w-3 shrink-0 text-red-400" />
                              ) : (
                                <CheckCircle2 className="h-3 w-3 shrink-0 text-emerald-400" />
                              )}
                              <span className="shrink-0 rounded border border-border-subtle px-1 text-[9px] uppercase text-muted-foreground">
                                {sr.type}
                              </span>
                              <span className="truncate font-medium">
                                {sr.title}
                              </span>
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
                                <p className="mt-1 text-[10px] text-red-400">
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
                    <p className="text-[10px] text-muted-foreground">
                      No step output recorded.
                    </p>
                  </div>
                )}
              </div>
            )
          })}
          {jobs.length === 0 && (
            <p className="p-2 text-[12px] text-muted-foreground">
              No runs yet.
            </p>
          )}
        </div>
      </ScrollArea>
    </div>
  )
}
