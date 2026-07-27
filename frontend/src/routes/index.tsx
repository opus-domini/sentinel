import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { RefreshCw } from 'lucide-react'
import type { NowResponse, NowServiceFailedAttention } from '@/types'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { NowAttention } from '@/components/now/NowAttention'
import { NowInProgress } from '@/components/now/NowInProgress'
import { NowReliability } from '@/components/now/NowReliability'
import { RunbookRunDialog } from '@/components/RunbookRunDialog'
import { Button } from '@/components/ui/button'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useOpsEvents, useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  NOW_QUERY_REFRESH_POLICY,
  markNowCurrentSourcesStale,
  shouldPresentNowSnapshotAsStale,
} from '@/lib/nowPresentation'
import { OPS_NOW_QUERY_KEY, OPS_RUNBOOKS_QUERY_KEY, isNowRelevantEvent } from '@/lib/opsQueryCache'

export const Route = createFileRoute('/')({
  component: IndexRoute,
})

function IndexRoute() {
  const { hostname } = useMetaContext()
  const { pushToast } = useToastContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()
  const forceReconnect = useOpsEventsReconnect()
  const [runTarget, setRunTarget] = useState<NowServiceFailedAttention | null>(null)
  const [startingProcedure, setStartingProcedure] = useState(false)
  const hasConnectedRef = useRef(false)
  const previousConnectionRef = useRef<string>('disconnected')

  const nowQuery = useQuery({
    queryKey: OPS_NOW_QUERY_KEY,
    queryFn: async () => {
      const response = await api<NowResponse>('/api/now')
      return response.now
    },
    ...NOW_QUERY_REFRESH_POLICY,
  })

  const handleEvent = useCallback(
    (message: unknown) => {
      if (!isNowRelevantEvent(message)) return
      void queryClient.invalidateQueries({ queryKey: OPS_NOW_QUERY_KEY, exact: true })
    },
    [queryClient],
  )
  const connectionState = useOpsEvents(handleEvent)
  const refetchNow = nowQuery.refetch

  useEffect(() => {
    const previous = previousConnectionRef.current
    previousConnectionRef.current = connectionState
    if (connectionState !== 'connected') return
    if (!hasConnectedRef.current) {
      hasConnectedRef.current = true
      return
    }
    if (previous !== 'connected') {
      void refetchNow()
    }
  }, [connectionState, refetchNow])

  const displaySnapshot = useMemo(() => {
    if (nowQuery.data == null) return null
    if (!shouldPresentNowSnapshotAsStale(connectionState, nowQuery.isRefetchError)) {
      return nowQuery.data
    }
    return markNowCurrentSourcesStale(nowQuery.data)
  }, [connectionState, nowQuery.data, nowQuery.isRefetchError])

  const resync = useCallback(() => {
    forceReconnect()
    void refetchNow()
  }, [forceReconnect, refetchNow])

  const runProcedure = useCallback(
    async (parameters: Record<string, string>) => {
      if (runTarget?.runbook == null) return
      setStartingProcedure(true)
      try {
        await api(`/api/now/services/${encodeURIComponent(runTarget.service.name)}/runbook`, {
          method: 'POST',
          body: JSON.stringify({ parameters }),
        })
        pushToast({
          level: 'success',
          title: 'Procedure started',
          message: runTarget.runbook.name,
        })
        setRunTarget(null)
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: OPS_NOW_QUERY_KEY, exact: true }),
          queryClient.invalidateQueries({ queryKey: OPS_RUNBOOKS_QUERY_KEY, exact: true }),
        ])
      } catch (error) {
        pushToast({
          level: 'error',
          title: 'Procedure not started',
          message: error instanceof Error ? error.message : 'failed to start procedure',
        })
      } finally {
        setStartingProcedure(false)
      }
    },
    [api, pushToast, queryClient, runTarget],
  )

  return (
    <AppShell>
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_17%_-8%,var(--section-glow-brand),transparent_32%),radial-gradient(circle_at_82%_8%,rgba(114,245,187,0.08),transparent_24%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <AppSectionTitle hostname={hostname} section="now" />
          </div>
          <ConnectionBadge state={connectionState} onClick={resync} actionLabel="Refresh Now" />
        </header>

        <div className="min-h-0 overflow-y-auto p-2 pb-4 sm:p-3 md:p-4">
          {nowQuery.isLoading && <NowLoading />}

          {nowQuery.isError && nowQuery.data == null && (
            <NowError
              message={
                nowQuery.error instanceof Error
                  ? nowQuery.error.message
                  : 'The operational snapshot could not be loaded.'
              }
              onRetry={() => void refetchNow()}
            />
          )}

          {displaySnapshot && (
            <div className="mx-auto grid w-full max-w-[1180px] gap-3 md:gap-4">
              <NowReliability snapshot={displaySnapshot} />
              <div className="grid min-h-0 gap-3 md:gap-4 lg:grid-cols-[minmax(0,1.65fr)_minmax(19rem,0.85fr)]">
                <NowAttention
                  attention={displaySnapshot.attention}
                  degraded={displaySnapshot.reliability.state === 'degraded'}
                  onRunProcedure={setRunTarget}
                />
                <NowInProgress
                  runs={displaySnapshot.inProgress.runs}
                  sessions={displaySnapshot.inProgress.sessions}
                />
              </div>
            </div>
          )}
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[10px] text-secondary-foreground">
          <span className="truncate">
            {displaySnapshot
              ? `${displaySnapshot.attention.total} attention · ${displaySnapshot.inProgress.runs.length} procedures · ${displaySnapshot.inProgress.sessions.length} sessions`
              : 'Operational read model'}
          </span>
          {nowQuery.isFetching && !nowQuery.isLoading && (
            <span className="inline-flex items-center gap-1 text-muted-foreground">
              <RefreshCw className="size-2.5 motion-safe:animate-spin" />
              Refreshing
            </span>
          )}
        </footer>
      </main>

      <RunbookRunDialog
        open={runTarget?.runbook != null}
        runbook={runTarget?.runbook ?? null}
        confirming={startingProcedure}
        onConfirm={(parameters) => void runProcedure(parameters)}
        onCancel={() => {
          if (!startingProcedure) setRunTarget(null)
        }}
      />
    </AppShell>
  )
}

function NowLoading() {
  return (
    <div aria-label="Loading Now" className="mx-auto grid w-full max-w-[1180px] gap-3 md:gap-4">
      <div className="h-56 motion-safe:animate-pulse rounded-xl border border-border-subtle bg-surface-raised" />
      <div className="grid gap-3 md:gap-4 lg:grid-cols-[minmax(0,1.65fr)_minmax(19rem,0.85fr)]">
        <div className="h-72 motion-safe:animate-pulse rounded-xl border border-border-subtle bg-secondary" />
        <div className="h-72 motion-safe:animate-pulse rounded-xl border border-border-subtle bg-surface-raised" />
      </div>
    </div>
  )
}

function NowError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="mx-auto grid min-h-[55vh] w-full max-w-2xl place-items-center rounded-xl border border-destructive/35 bg-destructive/6 p-6 text-center">
      <div>
        <p className="text-[14px] font-semibold text-destructive-foreground">Now is unavailable</p>
        <p className="mt-2 text-[11px] leading-relaxed text-secondary-foreground">{message}</p>
        <Button variant="outline" className="mt-4 cursor-pointer" onClick={onRetry}>
          <RefreshCw className="size-3" />
          Try again
        </Button>
      </div>
    </div>
  )
}
