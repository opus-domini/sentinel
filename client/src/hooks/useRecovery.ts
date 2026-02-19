import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type {
  RecoveryJob,
  RecoveryJobResponse,
  RecoveryOverviewResponse,
  RecoverySession,
  RecoverySnapshotResponse,
  RecoverySnapshotView,
  RecoverySnapshotsResponse,
} from '@/types'
import type {
  ApiFunction,
  RecoveryOverviewCache,
  RuntimeMetrics,
} from './tmuxTypes'
import { TMUX_RECOVERY_OVERVIEW_QUERY_KEY } from '@/lib/tmuxQueryCache'
import { slugifyTmuxName } from '@/lib/tmuxName'

type UseRecoveryOptions = {
  api: ApiFunction
  runtimeMetricsRef: React.MutableRefObject<RuntimeMetrics>
  refreshSessions: () => Promise<void>
  pushErrorToast: (title: string, message: string) => void
  pushSuccessToast: (title: string, message: string) => void
}

export function useRecovery(options: UseRecoveryOptions) {
  const {
    api,
    runtimeMetricsRef,
    refreshSessions,
    pushErrorToast,
    pushSuccessToast,
  } = options

  const queryClient = useQueryClient()
  const recoveryGenerationRef = useRef(0)

  const [recoverySessions, setRecoverySessions] = useState<
    Array<RecoverySession>
  >(
    () =>
      queryClient.getQueryData<RecoveryOverviewCache>(
        TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      )?.sessions ?? [],
  )
  const [recoveryJobs, setRecoveryJobs] = useState<Array<RecoveryJob>>(
    () =>
      queryClient.getQueryData<RecoveryOverviewCache>(
        TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      )?.jobs ?? [],
  )
  const [lastCollectAt, setLastCollectAt] = useState(
    () =>
      queryClient.getQueryData<RecoveryOverviewCache>(
        TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      )?.lastCollectAt ?? '',
  )
  const [recoveryDialogOpen, setRecoveryDialogOpen] = useState(false)
  const [recoverySnapshots, setRecoverySnapshots] = useState<
    Array<{ id: number; capturedAt: string; windows: number; panes: number }>
  >([])
  const [selectedRecoverySession, setSelectedRecoverySession] = useState<
    string | null
  >(null)
  const [selectedSnapshotID, setSelectedSnapshotID] = useState<number | null>(
    null,
  )
  const [selectedSnapshot, setSelectedSnapshot] =
    useState<RecoverySnapshotView | null>(null)
  const [recoveryLoading, setRecoveryLoading] = useState(false)
  const [recoveryBusy, setRecoveryBusy] = useState(false)
  const [recoveryError, setRecoveryError] = useState('')
  const [restoreMode, setRestoreMode] = useState<'safe' | 'confirm' | 'full'>(
    'confirm',
  )
  const [restoreConflictPolicy, setRestoreConflictPolicy] = useState<
    'rename' | 'replace' | 'skip'
  >('rename')
  const [restoreTargetSession, setRestoreTargetSession] = useState('')

  // Sync recovery cache
  useEffect(() => {
    queryClient.setQueryData<RecoveryOverviewCache>(
      TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
      {
        sessions: recoverySessions,
        jobs: recoveryJobs,
        lastCollectAt,
      },
    )
  }, [queryClient, recoveryJobs, recoverySessions, lastCollectAt])

  // Auto-select first recovery session
  useEffect(() => {
    if (recoverySessions.length === 0) {
      setSelectedRecoverySession(null)
      setRecoverySnapshots([])
      setSelectedSnapshot(null)
      setSelectedSnapshotID(null)
      return
    }
    if (
      selectedRecoverySession === null ||
      !recoverySessions.some((item) => item.name === selectedRecoverySession)
    ) {
      setSelectedRecoverySession(recoverySessions[0].name)
    }
  }, [recoverySessions, selectedRecoverySession])

  const refreshRecovery = useCallback(
    async (params?: { quiet?: boolean }) => {
      runtimeMetricsRef.current.recoveryRefreshCount += 1
      const gen = ++recoveryGenerationRef.current
      if (!params?.quiet) {
        setRecoveryLoading(true)
      }
      try {
        const data = await api<RecoveryOverviewResponse>(
          '/api/recovery/overview',
        )
        if (gen !== recoveryGenerationRef.current) return
        setRecoverySessions(data.overview.killedSessions)
        setRecoveryJobs(data.overview.runningJobs)
        setLastCollectAt(data.overview.lastCollectAt || '')
        queryClient.setQueryData<RecoveryOverviewCache>(
          TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
          {
            sessions: data.overview.killedSessions,
            jobs: data.overview.runningJobs,
            lastCollectAt: data.overview.lastCollectAt || '',
          },
        )
        setRecoveryError('')
      } catch (error) {
        if (gen !== recoveryGenerationRef.current) return
        const message =
          error instanceof Error ? error.message : 'failed to refresh recovery'
        if (message.toLowerCase().includes('recovery subsystem is disabled')) {
          setRecoverySessions([])
          setRecoveryJobs([])
          setLastCollectAt('')
          queryClient.setQueryData<RecoveryOverviewCache>(
            TMUX_RECOVERY_OVERVIEW_QUERY_KEY,
            {
              sessions: [],
              jobs: [],
              lastCollectAt: '',
            },
          )
          setRecoveryError('')
        } else {
          setRecoveryError(message)
        }
      } finally {
        if (gen === recoveryGenerationRef.current) {
          setRecoveryLoading(false)
        }
      }
    },
    [api, queryClient, runtimeMetricsRef],
  )

  const loadRecoverySnapshot = useCallback(
    async (snapshotID: number) => {
      setSelectedSnapshotID(snapshotID)
      try {
        const data = await api<RecoverySnapshotResponse>(
          `/api/recovery/snapshots/${snapshotID}`,
        )
        setSelectedSnapshot(data.snapshot)
        setRecoveryError('')
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to load snapshot'
        setRecoveryError(message)
      }
    },
    [api],
  )

  const loadRecoverySnapshots = useCallback(
    async (sessionName: string) => {
      const session = sessionName.trim()
      if (session === '') {
        setRecoverySnapshots([])
        setSelectedSnapshot(null)
        setSelectedSnapshotID(null)
        return
      }
      try {
        const data = await api<RecoverySnapshotsResponse>(
          `/api/recovery/sessions/${encodeURIComponent(session)}/snapshots?limit=50`,
        )
        const snapshots = data.snapshots.map((item) => ({
          id: item.id,
          capturedAt: item.capturedAt,
          windows: item.windows,
          panes: item.panes,
        }))
        setRecoverySnapshots(snapshots)
        if (snapshots.length > 0) {
          const first = snapshots[0]
          setRestoreTargetSession(session)
          await loadRecoverySnapshot(first.id)
        } else {
          setSelectedSnapshot(null)
          setSelectedSnapshotID(null)
        }
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to list snapshots'
        setRecoveryError(message)
      }
    },
    [api, loadRecoverySnapshot],
  )

  const pollRecoveryJob = useCallback(
    (jobID: string) => {
      const startedAt = Date.now()
      const maxDurationMs = 5 * 60 * 1000

      const tick = async () => {
        try {
          const data = await api<RecoveryJobResponse>(
            `/api/recovery/jobs/${encodeURIComponent(jobID)}`,
          )
          setRecoveryJobs((prev) => {
            const next = [data.job, ...prev.filter((j) => j.id !== data.job.id)]
            return next.slice(0, 30)
          })

          if (
            (data.job.status === 'queued' || data.job.status === 'running') &&
            Date.now() - startedAt < maxDurationMs
          ) {
            window.setTimeout(() => {
              void tick()
            }, 1200)
            return
          }

          setRecoveryBusy(false)
          if (data.job.status === 'succeeded') {
            pushSuccessToast(
              'Recovery',
              `session restored to "${data.job.targetSession || data.job.sessionName}"`,
            )
            await refreshSessions()
          } else if (data.job.status === 'failed') {
            pushErrorToast(
              'Recovery',
              data.job.error || 'restore job finished with errors',
            )
          }
          await refreshRecovery({ quiet: true })
        } catch (error) {
          setRecoveryBusy(false)
          const message =
            error instanceof Error
              ? error.message
              : 'failed to track restore progress'
          setRecoveryError(message)
        }
      }

      void tick()
    },
    [api, pushErrorToast, pushSuccessToast, refreshRecovery, refreshSessions],
  )

  const restoreSelectedSnapshot = useCallback(async () => {
    if (selectedSnapshotID === null) return
    setRecoveryBusy(true)
    setRecoveryError('')
    try {
      const data = await api<{ job: RecoveryJob }>(
        `/api/recovery/snapshots/${selectedSnapshotID}/restore`,
        {
          method: 'POST',
          body: JSON.stringify({
            mode: restoreMode,
            conflictPolicy: restoreConflictPolicy,
            targetSession: restoreTargetSession.trim(),
          }),
        },
      )
      setRecoveryJobs((prev) => [
        data.job,
        ...prev.filter((item) => item.id !== data.job.id),
      ])
      pollRecoveryJob(data.job.id)
    } catch (error) {
      setRecoveryBusy(false)
      const message =
        error instanceof Error ? error.message : 'failed to start restore'
      setRecoveryError(message)
      pushErrorToast('Recovery', message)
    }
  }, [
    api,
    pollRecoveryJob,
    pushErrorToast,
    restoreConflictPolicy,
    restoreMode,
    restoreTargetSession,
    selectedSnapshotID,
  ])

  const archiveRecoverySession = useCallback(
    async (sessionName: string) => {
      try {
        await api<void>(
          `/api/recovery/sessions/${encodeURIComponent(sessionName)}/archive`,
          {
            method: 'POST',
            body: '{}',
          },
        )
        await refreshRecovery({ quiet: true })
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'failed to archive session'
        setRecoveryError(message)
      }
    },
    [api, refreshRecovery],
  )

  // Auto-load snapshots when dialog opens and session is selected
  useEffect(() => {
    if (!recoveryDialogOpen || selectedRecoverySession === null) return
    void loadRecoverySnapshots(selectedRecoverySession)
  }, [loadRecoverySnapshots, recoveryDialogOpen, selectedRecoverySession])

  return {
    // State
    recoverySessions,
    recoveryJobs,
    lastCollectAt,
    recoveryDialogOpen,
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
    // Actions
    setRecoveryDialogOpen,
    setSelectedRecoverySession,
    setRestoreTargetSession: (session: string) =>
      setRestoreTargetSession(slugifyTmuxName(session)),
    setRestoreTargetSessionRaw: setRestoreTargetSession,
    setRestoreMode,
    setRestoreConflictPolicy,
    refreshRecovery,
    loadRecoverySnapshot,
    loadRecoverySnapshots,
    restoreSelectedSnapshot,
    archiveRecoverySession,
    pollRecoveryJob,
  }
}
