import { useCallback, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import cronstrue from 'cronstrue'
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  Menu,
  Pause,
  Pencil,
  Play,
  RefreshCw,
  Trash2,
  XCircle,
} from 'lucide-react'
import type {
  OpsRunbook,
  OpsRunbookRun,
  OpsRunbookRunResponse,
  OpsRunbooksResponse,
  OpsSchedule,
  OpsTimelineEvent,
} from '@/types'
import type { RunbookDraft } from '@/components/RunbookEditor'
import type { RunbookStepDraft } from '@/components/RunbookStepEditor'
import type { ScheduleDraft } from '@/components/RunbookScheduleEditor'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { RunbookDeleteDialog } from '@/components/RunbookDeleteDialog'
import { RunbookEditor, createBlankStep } from '@/components/RunbookEditor'
import { RunbookScheduleEditor } from '@/components/RunbookScheduleEditor'
import RunbooksSidebar from '@/components/RunbooksSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_RUNBOOKS_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
  upsertOpsRunbookJob,
} from '@/lib/opsQueryCache'
import { cn, randomId } from '@/lib/utils'

function isBuiltinRunbook(id: string): boolean {
  return id.startsWith('ops.')
}

function runbookToDraft(runbook: OpsRunbook): RunbookDraft {
  return {
    id: runbook.id,
    name: runbook.name,
    description: runbook.description,
    enabled: runbook.enabled,
    steps: runbook.steps.map(
      (step): RunbookStepDraft => ({
        key: randomId(),
        type: step.type as RunbookStepDraft['type'],
        title: step.title,
        command: step.command ?? '',
        check: step.check ?? '',
        description: step.description ?? '',
      }),
    ),
  }
}

function createBlankDraft(): RunbookDraft {
  return {
    id: null,
    name: '',
    description: '',
    enabled: true,
    steps: [createBlankStep()],
  }
}

function validateDraft(draft: RunbookDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (draft.name.trim() === '') {
    errors.name = 'Name is required'
  }
  if (draft.steps.length === 0) {
    errors.steps = 'At least one step is required'
  }
  draft.steps.forEach((step, i) => {
    if (step.title.trim() === '') {
      errors[`step.${i}.title`] = 'Title is required'
    }
    if (step.type === 'command' && step.command.trim() === '') {
      errors[`step.${i}.command`] = 'Command is required'
    }
    if (step.type === 'check' && step.check.trim() === '') {
      errors[`step.${i}.check`] = 'Check command is required'
    }
  })
  return errors
}

function draftToPayload(draft: RunbookDraft) {
  return {
    name: draft.name.trim(),
    description: draft.description.trim(),
    enabled: draft.enabled,
    steps: draft.steps.map((step) => {
      const base: Record<string, string> = {
        type: step.type,
        title: step.title.trim(),
      }
      if (step.type === 'command') base.command = step.command.trim()
      if (step.type === 'check') base.check = step.check.trim()
      if (step.type === 'manual' && step.description.trim() !== '')
        base.description = step.description.trim()
      return base
    }),
  }
}

function runbookJobStatusClass(status: string): string {
  const s = status.trim().toLowerCase()
  if (s === 'succeeded') return 'text-emerald-400'
  if (s === 'failed') return 'text-red-400'
  if (s === 'running') return 'text-amber-400'
  return 'text-muted-foreground'
}

function scheduleDescription(schedule: OpsSchedule): string {
  if (schedule.scheduleType === 'cron' && schedule.cronExpr) {
    try {
      return cronstrue.toString(schedule.cronExpr)
    } catch {
      return schedule.cronExpr
    }
  }
  if (schedule.scheduleType === 'once' && schedule.runAt) {
    return `Once at ${new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: schedule.timezone || undefined }).format(new Date(schedule.runAt))}`
  }
  return schedule.name
}

function formatScheduleDate(iso: string, tz?: string): string {
  if (!iso) return ''
  try {
    return new Intl.DateTimeFormat('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: tz || undefined,
      timeZoneName: 'short',
    }).format(new Date(iso))
  } catch {
    return iso
  }
}

function RunbooksPage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const { pushToast } = useToastContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [selectedRunbookId, setSelectedRunbookId] = useState<string | null>(
    null,
  )
  const [editingDraft, setEditingDraft] = useState<RunbookDraft | null>(null)
  const [saving, setSaving] = useState(false)
  const [editorErrors, setEditorErrors] = useState<Record<string, string>>({})
  const [deleteTarget, setDeleteTarget] = useState<OpsRunbook | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null)
  const [expandedStepIndices, setExpandedStepIndices] = useState<Set<number>>(
    new Set(),
  )
  const [deleteJobTarget, setDeleteJobTarget] = useState<string | null>(null)
  const [editingSchedule, setEditingSchedule] = useState<{
    runbookId: string
    schedule: OpsSchedule | null
  } | null>(null)
  const [scheduleSaving, setScheduleSaving] = useState(false)

  const runbooksQuery = useQuery({
    queryKey: OPS_RUNBOOKS_QUERY_KEY,
    queryFn: async () => {
      return api<OpsRunbooksResponse>('/api/ops/runbooks')
    },
  })

  const runbooks = runbooksQuery.data?.runbooks ?? []
  const jobs = runbooksQuery.data?.jobs ?? []
  const schedules = runbooksQuery.data?.schedules ?? []
  const runbooksLoading = runbooksQuery.isLoading

  const refreshRunbooks = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_RUNBOOKS_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      const typed = message as {
        type?: string
        payload?: {
          job?: OpsRunbookRun
        }
      }
      if (typed.type === 'ops.job.updated') {
        if (typed.payload?.job != null) {
          const job = typed.payload.job
          queryClient.setQueryData<OpsRunbooksResponse>(
            OPS_RUNBOOKS_QUERY_KEY,
            (previous) => {
              if (previous == null) return previous
              return {
                ...previous,
                jobs: upsertOpsRunbookJob(previous.jobs, job),
              }
            },
          )
        } else {
          void refreshRunbooks()
        }
      }
      if (typed.type === 'ops.schedule.updated') {
        void refreshRunbooks()
      }
    },
    [queryClient, refreshRunbooks],
  )

  const connectionState = useOpsEventsSocket({
    token,
    tokenRequired,
    onMessage: handleWSMessage,
  })

  const runRunbook = useCallback(
    async (runbookID: string) => {
      const runbook = runbooks.find((item) => item.id === runbookID)
      if (!runbook) return

      try {
        const data = await api<OpsRunbookRunResponse>(
          `/api/ops/runbooks/${encodeURIComponent(runbookID)}/run`,
          {
            method: 'POST',
          },
        )
        const job = data.job
        queryClient.setQueryData<OpsRunbooksResponse>(
          OPS_RUNBOOKS_QUERY_KEY,
          (previous) => {
            if (previous == null) return previous
            return {
              ...previous,
              jobs: upsertOpsRunbookJob(previous.jobs, job),
            }
          },
        )
        if (data.timelineEvent != null) {
          queryClient.setQueryData<Array<OpsTimelineEvent>>(
            opsTimelineQueryKey('', 'all'),
            (current = []) =>
              prependOpsTimelineEvent(
                current,
                data.timelineEvent as OpsTimelineEvent,
              ),
          )
        }
        pushToast({
          level: 'success',
          title: runbook.name,
          message: `run completed with status ${job.status}`,
        })
      } catch (error) {
        pushToast({
          level: 'error',
          title: runbook.name,
          message:
            error instanceof Error ? error.message : 'failed to run runbook',
        })
      }
    },
    [api, pushToast, queryClient, runbooks],
  )

  // --- Editor callbacks ---

  const startCreate = useCallback(() => {
    setEditingDraft(createBlankDraft())
    setEditorErrors({})
    setSelectedRunbookId(null)
  }, [])

  const startEdit = useCallback((runbook: OpsRunbook) => {
    setEditingDraft(runbookToDraft(runbook))
    setEditorErrors({})
  }, [])

  const cancelEdit = useCallback(() => {
    setEditingDraft(null)
    setEditorErrors({})
  }, [])

  const saveDraft = useCallback(async () => {
    if (editingDraft == null) return
    const errors = validateDraft(editingDraft)
    setEditorErrors(errors)
    if (Object.keys(errors).length > 0) return

    setSaving(true)
    try {
      const payload = draftToPayload(editingDraft)
      if (editingDraft.id == null) {
        const data = await api<{ runbook: OpsRunbook }>('/api/ops/runbooks', {
          method: 'POST',
          body: JSON.stringify(payload),
        })
        await refreshRunbooks()
        setSelectedRunbookId(data.runbook.id)
        pushToast({
          level: 'success',
          title: 'Runbook created',
          message: payload.name,
        })
      } else {
        await api(`/api/ops/runbooks/${encodeURIComponent(editingDraft.id)}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
        await refreshRunbooks()
        pushToast({
          level: 'success',
          title: 'Runbook updated',
          message: payload.name,
        })
      }
      setEditingDraft(null)
      setEditorErrors({})
    } catch (error) {
      pushToast({
        level: 'error',
        title: 'Save failed',
        message:
          error instanceof Error ? error.message : 'failed to save runbook',
      })
    } finally {
      setSaving(false)
    }
  }, [api, editingDraft, pushToast, refreshRunbooks])

  // --- Delete callbacks ---

  const confirmDelete = useCallback((runbook: OpsRunbook) => {
    setDeleteTarget(runbook)
  }, [])

  const cancelDelete = useCallback(() => {
    setDeleteTarget(null)
  }, [])

  const executeDelete = useCallback(async () => {
    if (deleteTarget == null) return
    setDeleting(true)
    try {
      await api(`/api/ops/runbooks/${encodeURIComponent(deleteTarget.id)}`, {
        method: 'DELETE',
      })
      await refreshRunbooks()
      pushToast({
        level: 'success',
        title: 'Runbook deleted',
        message: deleteTarget.name,
      })
      setSelectedRunbookId(null)
      setEditingDraft(null)
    } catch (error) {
      pushToast({
        level: 'error',
        title: 'Delete failed',
        message:
          error instanceof Error ? error.message : 'failed to delete runbook',
      })
    } finally {
      setDeleting(false)
      setDeleteTarget(null)
    }
  }, [api, deleteTarget, pushToast, refreshRunbooks])

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
      try {
        await api(`/api/ops/jobs/${encodeURIComponent(jobId)}`, {
          method: 'DELETE',
        })
        queryClient.setQueryData<OpsRunbooksResponse>(
          OPS_RUNBOOKS_QUERY_KEY,
          (previous) => {
            if (previous == null) return previous
            return {
              ...previous,
              jobs: previous.jobs.filter((j) => j.id !== jobId),
            }
          },
        )
        if (expandedJobId === jobId) setExpandedJobId(null)
        setDeleteJobTarget(null)
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Delete failed',
          message:
            error instanceof Error ? error.message : 'failed to delete job',
        })
      }
    },
    [api, expandedJobId, pushToast, queryClient],
  )

  const saveSchedule = useCallback(
    async (draft: ScheduleDraft) => {
      if (editingSchedule == null) return
      setScheduleSaving(true)
      try {
        const payload = {
          runbookId: editingSchedule.runbookId,
          name: draft.name.trim(),
          scheduleType: draft.scheduleType,
          cronExpr: draft.cronExpr,
          timezone: draft.timezone,
          runAt: draft.runAt,
          enabled: draft.enabled,
        }
        if (editingSchedule.schedule != null) {
          await api(
            `/api/ops/schedules/${encodeURIComponent(editingSchedule.schedule.id)}`,
            { method: 'PUT', body: JSON.stringify(payload) },
          )
          pushToast({
            level: 'success',
            title: 'Schedule updated',
            message: payload.name,
          })
        } else {
          await api('/api/ops/schedules', {
            method: 'POST',
            body: JSON.stringify(payload),
          })
          pushToast({
            level: 'success',
            title: 'Schedule created',
            message: payload.name,
          })
        }
        await refreshRunbooks()
        setEditingSchedule(null)
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Schedule save failed',
          message:
            error instanceof Error ? error.message : 'failed to save schedule',
        })
      } finally {
        setScheduleSaving(false)
      }
    },
    [api, editingSchedule, pushToast, refreshRunbooks],
  )

  const deleteSchedule = useCallback(
    async (scheduleId: string) => {
      try {
        await api(`/api/ops/schedules/${encodeURIComponent(scheduleId)}`, {
          method: 'DELETE',
        })
        await refreshRunbooks()
        setEditingSchedule(null)
        pushToast({
          level: 'success',
          title: 'Schedule deleted',
          message: 'Schedule removed',
        })
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Delete failed',
          message:
            error instanceof Error
              ? error.message
              : 'failed to delete schedule',
        })
      }
    },
    [api, pushToast, refreshRunbooks],
  )

  const toggleScheduleEnabled = useCallback(
    async (schedule: OpsSchedule) => {
      try {
        await api(`/api/ops/schedules/${encodeURIComponent(schedule.id)}`, {
          method: 'PUT',
          body: JSON.stringify({
            runbookId: schedule.runbookId,
            name: schedule.name,
            scheduleType: schedule.scheduleType,
            cronExpr: schedule.cronExpr,
            timezone: schedule.timezone,
            runAt: schedule.runAt,
            enabled: !schedule.enabled,
          }),
        })
        await refreshRunbooks()
        pushToast({
          level: 'success',
          title: schedule.enabled ? 'Schedule paused' : 'Schedule resumed',
          message: schedule.name,
        })
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Update failed',
          message:
            error instanceof Error
              ? error.message
              : 'failed to update schedule',
        })
      }
    },
    [api, pushToast, refreshRunbooks],
  )

  const triggerSchedule = useCallback(
    async (scheduleId: string) => {
      try {
        await api(
          `/api/ops/schedules/${encodeURIComponent(scheduleId)}/trigger`,
          { method: 'POST' },
        )
        await refreshRunbooks()
        pushToast({
          level: 'success',
          title: 'Schedule triggered',
          message: 'Manual trigger submitted',
        })
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Trigger failed',
          message:
            error instanceof Error
              ? error.message
              : 'failed to trigger schedule',
        })
      }
    },
    [api, pushToast, refreshRunbooks],
  )

  const selectedRunbook = useMemo(
    () => runbooks.find((rb) => rb.id === selectedRunbookId) ?? null,
    [runbooks, selectedRunbookId],
  )

  const selectedJobs = useMemo(
    () =>
      selectedRunbookId
        ? jobs
            .filter((j) => j.runbookId === selectedRunbookId)
            .sort(
              (a, b) =>
                new Date(b.createdAt).getTime() -
                new Date(a.createdAt).getTime(),
            )
        : [],
    [jobs, selectedRunbookId],
  )

  const selectedSchedule = useMemo(
    () =>
      selectedRunbookId
        ? (schedules.find((s) => s.runbookId === selectedRunbookId) ?? null)
        : null,
    [schedules, selectedRunbookId],
  )

  // Determine which view to render
  const showEditor = editingDraft != null
  const showDetail = !showEditor && selectedRunbook != null

  return (
    <AppShell
      sidebar={
        <RunbooksSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          loading={runbooksLoading}
          runbooks={runbooks}
          jobs={jobs}
          schedules={schedules}
          selectedRunbookId={selectedRunbookId}
          onTokenChange={setToken}
          onSelectRunbook={(id) => {
            setSelectedRunbookId(id)
            setEditingDraft(null)
            setEditorErrors({})
            setEditingSchedule(null)
          }}
          onCreateRunbook={startCreate}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(147,51,234,.16),transparent_34%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <span className="truncate">Sentinel</span>
            <span className="text-muted-foreground">/</span>
            <span className="truncate text-muted-foreground">runbooks</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={() => void refreshRunbooks()}
              aria-label="Refresh runbooks"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <div className="min-h-0 overflow-hidden p-3">
          {showEditor && (
            <RunbookEditor
              draft={editingDraft}
              saving={saving}
              errors={editorErrors}
              onDraftChange={setEditingDraft}
              onSave={() => void saveDraft()}
              onCancel={cancelEdit}
            />
          )}

          {showDetail && (
            <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden">
              <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <h2 className="truncate text-[14px] font-semibold">
                      {selectedRunbook.name}
                    </h2>
                    <p className="text-[12px] text-muted-foreground">
                      {selectedRunbook.description}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 cursor-pointer gap-1 px-3 text-[11px]"
                      onClick={() => startEdit(selectedRunbook)}
                    >
                      <Pencil className="h-3 w-3" />
                      Edit
                    </Button>
                    {isBuiltinRunbook(selectedRunbook.id) ? (
                      <TooltipHelper content="Built-in runbooks cannot be deleted">
                        <span>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-8 gap-1 px-3 text-[11px]"
                            disabled
                          >
                            <Trash2 className="h-3 w-3" />
                            Delete
                          </Button>
                        </span>
                      </TooltipHelper>
                    ) : (
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-8 cursor-pointer gap-1 px-3 text-[11px] text-red-400 hover:text-red-300"
                        onClick={() => confirmDelete(selectedRunbook)}
                      >
                        <Trash2 className="h-3 w-3" />
                        Delete
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 cursor-pointer gap-1 px-3 text-[11px]"
                      onClick={() => void runRunbook(selectedRunbook.id)}
                    >
                      <Play className="h-3 w-3" />
                      Run
                    </Button>
                  </div>
                </div>

                <div className="grid gap-1">
                  <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                    Steps ({selectedRunbook.steps.length})
                  </p>
                  {selectedRunbook.steps.map((step, i) => (
                    <div
                      key={i}
                      className="flex items-start gap-2 rounded border border-border-subtle bg-surface-overlay px-2 py-1.5 text-[11px]"
                    >
                      <span className="mt-0.5 shrink-0 rounded bg-surface-elevated px-1 text-[10px] text-muted-foreground">
                        {i + 1}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="rounded border border-border-subtle px-1 text-[9px] uppercase text-muted-foreground">
                            {step.type}
                          </span>
                          <span className="font-medium">{step.title}</span>
                        </div>
                        {step.command && (
                          <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                            {step.command}
                          </p>
                        )}
                      </div>
                    </div>
                  ))}
                </div>

                {/* Schedule section */}
                <div className="grid gap-1">
                  <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                    Schedule
                  </p>
                  {editingSchedule != null &&
                  editingSchedule.runbookId === selectedRunbook.id ? (
                    <RunbookScheduleEditor
                      runbookId={selectedRunbook.id}
                      schedule={editingSchedule.schedule}
                      saving={scheduleSaving}
                      onSave={(draft) => void saveSchedule(draft)}
                      onCancel={() => setEditingSchedule(null)}
                      onDelete={
                        editingSchedule.schedule
                          ? () =>
                              void deleteSchedule(editingSchedule.schedule!.id)
                          : undefined
                      }
                    />
                  ) : selectedSchedule ? (
                    <div className="rounded border border-border-subtle bg-surface-overlay px-2.5 py-2">
                      <div className="flex items-center gap-1.5">
                        <Clock className="h-3 w-3 text-muted-foreground" />
                        <span className="text-[11px] font-medium">
                          {scheduleDescription(selectedSchedule)}
                        </span>
                        <span className="text-[10px] text-muted-foreground">
                          ({selectedSchedule.timezone})
                        </span>
                        {!selectedSchedule.enabled && (
                          <span className="rounded bg-amber-500/20 px-1 text-[9px] font-medium text-amber-400">
                            paused
                          </span>
                        )}
                      </div>
                      {selectedSchedule.nextRunAt && (
                        <p className="mt-0.5 text-[10px] text-muted-foreground">
                          Next:{' '}
                          {formatScheduleDate(
                            selectedSchedule.nextRunAt,
                            selectedSchedule.timezone,
                          )}
                        </p>
                      )}
                      {selectedSchedule.lastRunAt && (
                        <p className="text-[10px] text-muted-foreground">
                          Last:{' '}
                          {formatScheduleDate(
                            selectedSchedule.lastRunAt,
                            selectedSchedule.timezone,
                          )}
                          {selectedSchedule.lastRunStatus &&
                            ` \u00b7 ${selectedSchedule.lastRunStatus}`}
                        </p>
                      )}
                      <div className="mt-1.5 flex items-center gap-1">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                          onClick={() =>
                            setEditingSchedule({
                              runbookId: selectedRunbook.id,
                              schedule: selectedSchedule,
                            })
                          }
                        >
                          <Pencil className="h-2.5 w-2.5" />
                          Edit
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                          onClick={() =>
                            void toggleScheduleEnabled(selectedSchedule)
                          }
                        >
                          {selectedSchedule.enabled ? (
                            <>
                              <Pause className="h-2.5 w-2.5" />
                              Pause
                            </>
                          ) : (
                            <>
                              <Play className="h-2.5 w-2.5" />
                              Resume
                            </>
                          )}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-6 cursor-pointer gap-1 px-2 text-[10px]"
                          onClick={() =>
                            void triggerSchedule(selectedSchedule.id)
                          }
                        >
                          <Play className="h-2.5 w-2.5" />
                          Trigger
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-6 cursor-pointer gap-1 px-2 text-[10px] text-red-400 hover:text-red-300"
                          onClick={() =>
                            void deleteSchedule(selectedSchedule.id)
                          }
                        >
                          <Trash2 className="h-2.5 w-2.5" />
                          Delete
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-fit cursor-pointer gap-1 px-3 text-[11px]"
                      onClick={() =>
                        setEditingSchedule({
                          runbookId: selectedRunbook.id,
                          schedule: null,
                        })
                      }
                    >
                      <Clock className="h-3 w-3" />
                      Add Schedule
                    </Button>
                  )}
                </div>
              </div>

              <div className="grid min-h-0 grid-rows-[auto_1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
                <div className="flex items-center justify-between border-b border-border-subtle px-3 py-2">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                    Job History
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    {selectedJobs.length} runs
                  </span>
                </div>
                <ScrollArea className="h-full min-h-0">
                  <div className="grid gap-1 p-2">
                    {selectedJobs.map((job) => {
                      const isExpanded = expandedJobId === job.id
                      const steps = job.stepResults
                      return (
                        <div
                          key={job.id}
                          className="group/job rounded border border-border-subtle bg-surface-elevated"
                        >
                          <div className="flex items-start gap-1.5 px-2.5 py-2">
                            <button
                              type="button"
                              className="mt-0.5 shrink-0 cursor-pointer text-muted-foreground"
                              onClick={() => toggleJobExpand(job.id)}
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
                              <p className="text-[10px] text-muted-foreground">
                                {job.createdAt}
                                {job.currentStep && ` · ${job.currentStep}`}
                              </p>
                              {job.error && (
                                <p className="mt-1 text-[10px] text-red-400">
                                  {job.error}
                                </p>
                              )}
                            </div>
                            {job.status !== 'running' && (
                              <button
                                type="button"
                                className="mt-0.5 shrink-0 cursor-pointer text-muted-foreground opacity-0 transition-opacity hover:text-red-400 group-hover/job:opacity-100"
                                onClick={() =>
                                  setDeleteJobTarget(
                                    deleteJobTarget === job.id ? null : job.id,
                                  )
                                }
                                aria-label="Delete job"
                              >
                                <Trash2 className="h-3 w-3" />
                              </button>
                            )}
                          </div>
                          {deleteJobTarget === job.id && (
                            <div className="flex items-center gap-2 border-t border-border-subtle px-2.5 py-1.5">
                              <span className="text-[10px] text-muted-foreground">
                                Delete this run?
                              </span>
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-5 cursor-pointer px-2 text-[10px] text-red-400 hover:text-red-300"
                                onClick={() => void deleteJob(job.id)}
                              >
                                Confirm
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-5 cursor-pointer px-2 text-[10px]"
                                onClick={() => setDeleteJobTarget(null)}
                              >
                                Cancel
                              </Button>
                            </div>
                          )}
                          {isExpanded && steps.length > 0 && (
                            <div className="grid gap-0.5 border-t border-border-subtle px-2.5 py-2">
                              {steps.map((sr) => {
                                const stepOpen = expandedStepIndices.has(
                                  sr.stepIndex,
                                )
                                return (
                                  <div
                                    key={sr.stepIndex}
                                    className="rounded border border-border-subtle bg-surface-overlay"
                                  >
                                    <button
                                      type="button"
                                      className="flex w-full cursor-pointer items-center justify-between gap-2 px-2 py-1.5 text-[11px]"
                                      onClick={() =>
                                        toggleStepExpand(sr.stepIndex)
                                      }
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
                                          <pre className="max-h-40 overflow-y-auto whitespace-pre-wrap rounded bg-[var(--background)] px-2 py-1 font-mono text-[10px] text-secondary-foreground">
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
                    {selectedJobs.length === 0 && (
                      <p className="p-2 text-[12px] text-muted-foreground">
                        No runs yet.
                      </p>
                    )}
                  </div>
                </ScrollArea>
              </div>
            </div>
          )}

          {!showEditor && !showDetail && (
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-2">
                {runbooks.map((runbook) => {
                  const lastJob = jobs.find(
                    (job) => job.runbookId === runbook.id,
                  )
                  return (
                    <button
                      key={runbook.id}
                      type="button"
                      className="grid cursor-pointer gap-2 rounded-lg border border-border-subtle bg-surface-elevated px-3 py-2.5 text-left transition-colors hover:border-border hover:bg-surface-overlay"
                      onClick={() => setSelectedRunbookId(runbook.id)}
                    >
                      <div className="flex min-w-0 items-center justify-between gap-2">
                        <div className="min-w-0">
                          <p className="truncate text-[12px] font-semibold">
                            {runbook.name}
                          </p>
                          <p className="text-[11px] text-muted-foreground">
                            {runbook.description}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 cursor-pointer text-[11px]"
                          onClick={(e) => {
                            e.stopPropagation()
                            void runRunbook(runbook.id)
                          }}
                        >
                          Run
                        </Button>
                      </div>
                      <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
                        <span>{runbook.steps.length} steps</span>
                        {schedules.some((s) => s.runbookId === runbook.id) && (
                          <span className="inline-flex items-center gap-0.5 rounded border border-border-subtle px-1 text-[9px]">
                            <Clock className="h-2.5 w-2.5" />
                            Scheduled
                          </span>
                        )}
                      </div>
                      <div className="rounded border border-border-subtle bg-surface-overlay px-2 py-1 text-[10px] text-muted-foreground">
                        {lastJob
                          ? `last run: ${lastJob.status} · ${lastJob.completedSteps}/${lastJob.totalSteps} · ${lastJob.createdAt}`
                          : 'never ran'}
                      </div>
                    </button>
                  )
                })}
                {!runbooksLoading && runbooks.length === 0 && (
                  <div className="rounded-lg border border-dashed border-border-subtle p-6 text-center">
                    <p className="text-[12px] text-muted-foreground">
                      No runbooks available.
                    </p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="mt-2 h-7 cursor-pointer text-[11px]"
                      onClick={startCreate}
                    >
                      Create your first runbook
                    </Button>
                  </div>
                )}
              </div>
            </ScrollArea>
          )}
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">
            {runbooks.length} runbooks
          </span>
        </footer>
      </main>

      <RunbookDeleteDialog
        open={deleteTarget != null}
        runbookName={deleteTarget?.name ?? ''}
        deleting={deleting}
        onConfirm={() => void executeDelete()}
        onCancel={cancelDelete}
      />
    </AppShell>
  )
}

export const Route = createFileRoute('/runbooks')({
  component: RunbooksPage,
})
