import { useCallback, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  OpsActivityEvent,
  OpsRunbook,
  OpsRunbookRunResponse,
  OpsRunbooksResponse,
  OpsSchedule,
  OpsWsMessage,
} from '@/types'
import type { RunbookDraft } from '@/components/RunbookEditor'
import type { RunbookStepDraft } from '@/components/RunbookStepEditor'
import type { ScheduleDraft } from '@/components/RunbookScheduleEditor'
import { createBlankStep } from '@/components/RunbookEditor'
import { useToastContext } from '@/contexts/ToastContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_RUNBOOKS_QUERY_KEY,
  opsActivityQueryKey,
  prependOpsActivityEvent,
  upsertOpsRunbookJob,
} from '@/lib/opsQueryCache'
import { randomId } from '@/lib/utils'

function runbookToDraft(runbook: OpsRunbook): RunbookDraft {
  return {
    id: runbook.id,
    name: runbook.name,
    description: runbook.description,
    enabled: runbook.enabled,
    webhookURL: runbook.webhookURL ?? '',
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
    webhookURL: '',
    steps: [createBlankStep()],
  }
}

function validateDraft(draft: RunbookDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (draft.name.trim() === '') {
    errors.name = 'Name is required'
  }
  const trimmedURL = draft.webhookURL.trim()
  if (trimmedURL !== '') {
    try {
      const u = new URL(trimmedURL)
      if (u.protocol !== 'http:' && u.protocol !== 'https:') {
        errors.webhookURL = 'Must use http or https'
      }
    } catch {
      errors.webhookURL = 'Invalid URL'
    }
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
    webhookURL: draft.webhookURL.trim(),
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

type UseRunbooksPageOptions = {
  authenticated: boolean
  tokenRequired: boolean
}

export function useRunbooksPage({
  authenticated,
  tokenRequired,
}: UseRunbooksPageOptions) {
  const { pushToast } = useToastContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [selectedRunbookId, setSelectedRunbookId] = useState<string | null>(
    null,
  )
  const [editingDraft, setEditingDraft] = useState<RunbookDraft | null>(null)
  const [saving, setSaving] = useState(false)
  const [editorErrors, setEditorErrors] = useState<Record<string, string>>({})
  const [deleteTarget, setDeleteTarget] = useState<OpsRunbook | null>(null)
  const [deleting, setDeleting] = useState(false)

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
      const msg = message as OpsWsMessage
      switch (msg.type) {
        case 'ops.job.updated': {
          const job = msg.payload.job
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
          break
        }
        case 'ops.schedule.updated':
          void refreshRunbooks()
          break
        default:
          break
      }
    },
    [queryClient, refreshRunbooks],
  )

  const connectionState = useOpsEventsSocket({
    authenticated,
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
          queryClient.setQueryData<Array<OpsActivityEvent>>(
            opsActivityQueryKey('', 'all'),
            (current = []) =>
              prependOpsActivityEvent(
                current,
                data.timelineEvent as OpsActivityEvent,
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
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Delete failed',
          message:
            error instanceof Error ? error.message : 'failed to delete job',
        })
      }
    },
    [api, pushToast, queryClient],
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

  const selectRunbook = useCallback((id: string) => {
    setSelectedRunbookId(id)
    setEditingDraft(null)
    setEditorErrors({})
    setEditingSchedule(null)
  }, [])

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

  return {
    // Data
    runbooks,
    jobs,
    schedules,
    runbooksLoading,
    connectionState,
    selectedRunbookId,
    selectedRunbook,
    selectedJobs,
    selectedSchedule,

    // Editor state
    editingDraft,
    setEditingDraft,
    saving,
    editorErrors,

    // Delete state
    deleteTarget,
    deleting,

    // Schedule state
    editingSchedule,
    setEditingSchedule,
    scheduleSaving,

    // Actions
    refreshRunbooks,
    runRunbook,
    startCreate,
    startEdit,
    cancelEdit,
    saveDraft,
    confirmDelete,
    cancelDelete,
    executeDelete,
    deleteJob,
    saveSchedule,
    deleteSchedule,
    toggleScheduleEnabled,
    triggerSchedule,
    selectRunbook,
  }
}
