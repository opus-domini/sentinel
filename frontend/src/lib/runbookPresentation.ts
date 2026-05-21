import type { OpsRunbook, OpsRunbookRun } from '@/types'

export type RunbookStatusTone =
  | 'ok'
  | 'danger'
  | 'warning'
  | 'muted'
  | 'neutral'

export type RunbookStatusMeta = {
  label: string
  tone: RunbookStatusTone
  dotClass: string
  textClass: string
}

function jobTime(job: OpsRunbookRun): number {
  const raw = job.createdAt || job.startedAt || job.finishedAt || ''
  const parsed = Date.parse(raw)
  return Number.isNaN(parsed) ? 0 : parsed
}

export function normalizeRunbookStatus(status: string): string {
  return status.trim().toLowerCase()
}

export function isActiveRunbookJob(
  job: Pick<OpsRunbookRun, 'status'>,
): boolean {
  const status = normalizeRunbookStatus(job.status)
  return (
    status === 'queued' || status === 'running' || status === 'waiting_approval'
  )
}

export function isExecutingRunbookJob(
  job: Pick<OpsRunbookRun, 'status'>,
): boolean {
  const status = normalizeRunbookStatus(job.status)
  return status === 'queued' || status === 'running'
}

export function isWaitingApprovalRunbookJob(
  job: Pick<OpsRunbookRun, 'status'>,
): boolean {
  return normalizeRunbookStatus(job.status) === 'waiting_approval'
}

export function latestRunbookJob(
  runbookId: string,
  jobs: Array<OpsRunbookRun>,
): OpsRunbookRun | null {
  return (
    jobs
      .filter((job) => job.runbookId === runbookId)
      .slice()
      .sort((a, b) => jobTime(b) - jobTime(a))[0] ?? null
  )
}

export function runbookStatusMeta(
  runbook: OpsRunbook,
  jobs: Array<OpsRunbookRun>,
): RunbookStatusMeta {
  if (!runbook.enabled) {
    return {
      label: 'Disabled',
      tone: 'muted',
      dotClass: 'bg-muted-foreground/50',
      textClass: 'text-muted-foreground',
    }
  }

  const lastJob = latestRunbookJob(runbook.id, jobs)
  if (!lastJob) {
    return {
      label: 'Not run',
      tone: 'neutral',
      dotClass: 'bg-muted-foreground/50',
      textClass: 'text-muted-foreground',
    }
  }

  const status = normalizeRunbookStatus(lastJob.status)
  if (status === 'succeeded') {
    return {
      label: 'Healthy',
      tone: 'ok',
      dotClass: 'bg-ok',
      textClass: 'text-ok-foreground',
    }
  }
  if (status === 'failed') {
    return {
      label: 'Failed',
      tone: 'danger',
      dotClass: 'bg-destructive',
      textClass: 'text-destructive-foreground',
    }
  }
  if (status === 'running') {
    return {
      label: 'Running',
      tone: 'warning',
      dotClass: 'bg-warning',
      textClass: 'text-warning-foreground',
    }
  }
  if (status === 'queued') {
    return {
      label: 'Queued',
      tone: 'warning',
      dotClass: 'bg-warning',
      textClass: 'text-warning-foreground',
    }
  }
  if (status === 'waiting_approval') {
    return {
      label: 'Waiting approval',
      tone: 'warning',
      dotClass: 'bg-warning',
      textClass: 'text-warning-foreground',
    }
  }

  return {
    label: lastJob.status || 'Unknown',
    tone: 'muted',
    dotClass: 'bg-muted-foreground/50',
    textClass: 'text-muted-foreground',
  }
}

export function runbookSearchText(runbook: OpsRunbook): string {
  return [
    runbook.name,
    runbook.description,
    runbook.steps
      .flatMap((step) => [
        step.type,
        step.title,
        step.command ?? '',
        step.script ?? '',
        step.description ?? '',
      ])
      .join(' '),
    (runbook.parameters ?? [])
      .flatMap((param) => [
        param.name,
        param.label,
        param.type,
        param.default,
        ...(param.options ?? []),
      ])
      .join(' '),
  ]
    .join(' ')
    .toLowerCase()
}

export function runbookJobProgress(job: OpsRunbookRun): number {
  if (job.totalSteps <= 0) {
    return isActiveRunbookJob(job) ? 0 : 100
  }
  return Math.min(100, Math.max(0, (job.completedSteps / job.totalSteps) * 100))
}

export function runbookJobDurationMs(
  job: OpsRunbookRun,
  now: Date = new Date(),
): number | null {
  const startedAt = Date.parse(job.startedAt || job.createdAt)
  if (Number.isNaN(startedAt)) {
    return null
  }

  if (job.finishedAt) {
    const finishedAt = Date.parse(job.finishedAt)
    if (!Number.isNaN(finishedAt) && finishedAt >= startedAt) {
      return finishedAt - startedAt
    }
  }

  if (isActiveRunbookJob(job)) {
    const current = now.getTime()
    return current >= startedAt ? current - startedAt : null
  }

  const stepDuration = job.stepResults.reduce(
    (total, step) => total + Math.max(0, step.durationMs),
    0,
  )
  return stepDuration > 0 ? stepDuration : null
}

export function formatRunbookDuration(ms: number | null): string {
  if (ms == null) {
    return 'n/a'
  }
  if (ms < 1000) {
    return `${Math.max(0, Math.round(ms))}ms`
  }

  const seconds = Math.round(ms / 1000)
  if (seconds < 60) {
    return `${seconds}s`
  }

  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  if (minutes < 60) {
    return remainingSeconds > 0
      ? `${minutes}m ${remainingSeconds}s`
      : `${minutes}m`
  }

  const hours = Math.floor(minutes / 60)
  const remainingMinutes = minutes % 60
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`
}
