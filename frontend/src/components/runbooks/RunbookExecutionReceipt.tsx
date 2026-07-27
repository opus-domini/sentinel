import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, FileClock, Loader2, ServerCog } from 'lucide-react'
import type {
  OpsRunbookRun,
  OpsRunbookStep,
  OpsRunbookStepResult,
  OpsServiceStatusResponse,
} from '@/types'
import { Badge } from '@/components/ui/badge'
import { useDateFormat } from '@/hooks/useDateFormat'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { serviceStatusSearch } from '@/lib/deepLinks'
import { cn } from '@/lib/utils'

type RunbookExecutionReceiptProps = {
  job: OpsRunbookRun
  standalone?: boolean
}

function resultTone(status: string): string {
  if (status === 'succeeded') return 'text-ok-foreground'
  if (status === 'failed') return 'text-destructive-foreground'
  return 'text-warning-foreground'
}

function sourceLabel(source?: OpsRunbookRun['source']): string {
  switch (source) {
    case 'now':
      return 'Now'
    case 'scheduler':
      return 'Scheduler'
    case 'runbooks':
      return 'Runbooks'
    default:
      return 'Legacy / unknown'
  }
}

function stepDefinitionText(step: OpsRunbookStep): string {
  if (step.type === 'run') return step.command ?? ''
  if (step.type === 'script') return step.script ?? ''
  return step.description ?? ''
}

function receiptStepState(
  index: number,
  completedSteps: number,
  result?: OpsRunbookStepResult,
): string {
  if (result?.error) return 'Failed'
  if (result != null) return 'Recorded'
  if (index < completedSteps) return 'Completed'
  return 'Pending'
}

export function RunbookExecutionReceipt({ job, standalone = false }: RunbookExecutionReceiptProps) {
  const api = useTmuxApi()
  const { formatDateTime, formatRelativeTime } = useDateFormat()
  const targetKind = job.targetKind || job.definition?.targetKind
  const targetName =
    targetKind === 'service'
      ? job.targetName?.trim() || job.definition?.targetName?.trim() || ''
      : ''
  const targetQuery = useQuery({
    queryKey: ['ops', 'service-status', targetName],
    queryFn: async () => {
      const data = await api<OpsServiceStatusResponse>(
        `/api/ops/services/${encodeURIComponent(targetName)}/status`,
      )
      return data.status
    },
    enabled: targetName !== '',
    retry: false,
  })
  const resultsByStep = useMemo(
    () => new Map(job.stepResults.map((result) => [result.stepIndex, result])),
    [job.stepResults],
  )
  const frozenSteps = useMemo(() => {
    const occurrences = new Map<string, number>()
    return (job.definition?.steps ?? []).map((step, index) => {
      const signature = JSON.stringify(step)
      const occurrence = occurrences.get(signature) ?? 0
      occurrences.set(signature, occurrence + 1)
      return { key: `${signature}:${occurrence}`, index, step }
    })
  }, [job.definition?.steps])
  const parameters = Object.entries(job.parametersUsed ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  )

  return (
    <section
      aria-label={`Execution receipt ${job.id}`}
      className={cn(
        'grid min-w-0 gap-3 border-border-subtle bg-surface-sunken',
        standalone ? 'rounded-lg border p-3 sm:p-4' : 'border-t px-2.5 py-3',
      )}
    >
      <header className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="flex items-center gap-1.5 text-[10px] uppercase tracking-[0.08em] text-primary-text">
            <FileClock className="size-3" />
            Immutable execution receipt
          </p>
          <h2 className="mt-1 truncate text-[13px] font-semibold text-foreground">
            {job.definition?.name || job.runbookName}
          </h2>
          <p className="truncate font-mono text-[9px] text-muted-foreground">{job.id}</p>
        </div>
        <Badge variant="outline" className="shrink-0 text-[9px]">
          {job.definition ? `Schema ${job.definition.schemaVersion}` : 'Legacy receipt'}
        </Badge>
      </header>

      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <ReceiptFact label="Execution result">
          <span className={cn('font-semibold', resultTone(job.status))}>{job.status}</span>
        </ReceiptFact>
        <ReceiptFact label="Origin">{sourceLabel(job.source)}</ReceiptFact>
        <ReceiptFact label="Created">{formatDateTime(job.createdAt)}</ReceiptFact>
        <ReceiptFact label="Finished">
          {job.finishedAt ? formatDateTime(job.finishedAt) : 'Not finished'}
        </ReceiptFact>
      </div>

      {job.startedAt && (
        <p className="text-[9px] text-muted-foreground">
          Started {formatDateTime(job.startedAt)}
          {job.finishedAt ? ` · Finished ${formatRelativeTime(job.finishedAt)}` : ''}
        </p>
      )}

      {targetName && (
        <div className="grid gap-2 rounded border border-border-subtle bg-surface-overlay p-2.5 sm:grid-cols-2">
          <div className="min-w-0">
            <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
              Recorded target
            </p>
            <Link
              to="/services"
              search={serviceStatusSearch(targetName)}
              className="mt-1 inline-flex min-w-0 items-center gap-1 font-mono text-[11px] text-primary-text no-underline hover:underline"
            >
              <ServerCog className="size-3 shrink-0" />
              <span className="truncate">{targetName}</span>
              <ArrowUpRight className="size-3 shrink-0" />
            </Link>
          </div>
          <div className="min-w-0">
            <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
              Current target state
            </p>
            {targetQuery.isLoading && (
              <p className="mt-1 flex items-center gap-1 text-[10px] text-muted-foreground">
                <Loader2 className="size-3 motion-safe:animate-spin" />
                Checking current state
              </p>
            )}
            {targetQuery.isError && (
              <p className="mt-1 text-[10px] text-warning-foreground">
                Current target state unavailable
              </p>
            )}
            {targetQuery.data && (
              <>
                <p className="mt-1 text-[11px] font-semibold text-foreground">
                  {targetQuery.data.service.activeState || 'unknown'}
                </p>
                <p className="text-[9px] text-muted-foreground">
                  Observed {formatRelativeTime(targetQuery.data.observedAt)}
                </p>
              </>
            )}
          </div>
        </div>
      )}

      {parameters.length > 0 && (
        <div className="grid gap-1">
          <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
            Persisted parameter values
          </p>
          <div className="flex flex-wrap gap-1">
            {parameters.map(([name, value]) => (
              <Badge key={name} variant="outline" className="h-auto max-w-full gap-1 text-[9px]">
                <span className="font-mono">{name}</span>
                <span className="text-muted-foreground">=</span>
                <span className="truncate">{value}</span>
              </Badge>
            ))}
          </div>
        </div>
      )}

      {job.definition == null ? (
        <div className="rounded border border-warning/30 bg-warning/6 p-2.5">
          <p className="text-[10px] font-semibold text-warning-foreground">
            Definition snapshot unavailable
          </p>
          <p className="mt-1 text-[9px] leading-relaxed text-muted-foreground">
            This legacy execution predates immutable receipts. Persisted result and timestamps
            remain readable, but its original steps cannot be reconstructed.
          </p>
        </div>
      ) : (
        <div className="grid gap-1.5">
          <div className="flex items-center justify-between gap-2">
            <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
              Frozen steps
            </p>
            <span className="text-[9px] text-muted-foreground">
              {job.completedSteps}/{job.definition.steps.length} completed
            </span>
          </div>
          {frozenSteps.map(({ key, index, step }) => {
            const result = resultsByStep.get(index)
            const definitionText = stepDefinitionText(step)
            return (
              <article
                key={key}
                className="grid min-w-0 gap-1 rounded border border-border-subtle bg-surface-overlay p-2"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="shrink-0 font-mono text-[9px] text-muted-foreground">
                    {index + 1}
                  </span>
                  <Badge variant="outline" className="h-4 shrink-0 px-1 text-[8px] uppercase">
                    {step.type}
                  </Badge>
                  <span className="min-w-0 flex-1 truncate text-[10px] font-medium">
                    {step.title}
                  </span>
                  <span className="shrink-0 text-[9px] text-muted-foreground">
                    {receiptStepState(index, job.completedSteps, result)}
                  </span>
                </div>
                {definitionText && (
                  <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-words rounded bg-background px-2 py-1 font-mono text-[9px] text-secondary-foreground">
                    {definitionText}
                  </pre>
                )}
                {result && (
                  <div className="grid gap-1 border-t border-border-subtle pt-1.5">
                    <p className="text-[9px] text-muted-foreground">
                      Result · {result.durationMs}ms
                    </p>
                    {result.output && (
                      <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words rounded bg-background px-2 py-1 font-mono text-[9px] text-secondary-foreground">
                        {result.output}
                      </pre>
                    )}
                    {result.error && (
                      <p className="text-[9px] text-destructive-foreground">{result.error}</p>
                    )}
                  </div>
                )}
              </article>
            )
          })}
        </div>
      )}

      {job.error && (
        <div className="rounded border border-destructive/30 bg-destructive/6 px-2.5 py-2">
          <p className="text-[9px] uppercase tracking-[0.08em] text-destructive-foreground">
            Execution error
          </p>
          <p className="mt-1 text-[10px] text-destructive-foreground">{job.error}</p>
        </div>
      )}
    </section>
  )
}

function ReceiptFact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
      <span className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">{label}</span>
      <span className="truncate text-[10px] text-foreground">{children}</span>
    </div>
  )
}
