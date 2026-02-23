import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, RefreshCw } from 'lucide-react'
import type {
  OpsActivityEvent,
  OpsActivityResponse,
  OpsOverviewResponse,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { TooltipHelper } from '@/components/TooltipHelper'
import ActivitiesSidebar from '@/components/ActivitiesSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useDateFormat } from '@/hooks/useDateFormat'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_OVERVIEW_QUERY_KEY,
  isOpsWsMessage,
  opsActivityQueryKey,
  prependOpsActivityEvent,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'

type ActivitiesFooterSummaryParams = {
  overviewError: string
  activityError: string
  overviewLoading: boolean
  activityLoading: boolean
  eventCount: number
}

function buildActivitiesFooterSummary({
  overviewError,
  activityError,
  overviewLoading,
  activityLoading,
  eventCount,
}: ActivitiesFooterSummaryParams): string {
  if (overviewError.trim() !== '') {
    return overviewError
  }
  if (activityError.trim() !== '') {
    return activityError
  }
  if (overviewLoading || activityLoading) {
    return 'Loading activities...'
  }
  return `${eventCount} events`
}

function ActivitiesPage() {
  const { tokenRequired } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const { formatDateTime } = useDateFormat()
  const layout = useLayoutContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [activityQuery, setActivityQuery] = useState('')
  const [activitySeverity, setActivitySeverity] = useState('all')

  const activityQueryKey = useMemo(
    () => opsActivityQueryKey(activityQuery, activitySeverity),
    [activityQuery, activitySeverity],
  )
  const activityQueryRef = useRef(activityQuery)
  const activitySeverityRef = useRef(activitySeverity)
  useEffect(() => {
    activityQueryRef.current = activityQuery
  }, [activityQuery])
  useEffect(() => {
    activitySeverityRef.current = activitySeverity
  }, [activitySeverity])

  const overviewQuery = useQuery({
    queryKey: OPS_OVERVIEW_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsOverviewResponse>('/api/ops/overview')
      return data.overview
    },
  })

  const activityEventsQuery = useQuery({
    queryKey: activityQueryKey,
    queryFn: async () => {
      const params = new URLSearchParams({ limit: '200' })
      if (activityQuery.trim() !== '') params.set('q', activityQuery.trim())
      if (activitySeverity !== 'all') params.set('severity', activitySeverity)
      const data = await api<OpsActivityResponse>(
        `/api/ops/activity?${params.toString()}`,
      )
      return data.events
    },
  })

  const overview = overviewQuery.data ?? null
  const activityEvents = activityEventsQuery.data ?? []
  const overviewLoading = overviewQuery.isLoading
  const activityLoading = activityEventsQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const activityError =
    activityEventsQuery.error != null
      ? toErrorMessage(activityEventsQuery.error, 'failed to load activities')
      : ''

  const refreshOverview = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_OVERVIEW_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshActivity = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: opsActivityQueryKey(
        activityQueryRef.current,
        activitySeverityRef.current,
      ),
      exact: true,
    })
  }, [queryClient])

  const refreshPage = useCallback(() => {
    void refreshOverview()
    void refreshActivity()
  }, [refreshOverview, refreshActivity])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      if (!isOpsWsMessage(message)) return
      switch (message.type) {
        case 'ops.overview.updated':
          queryClient.setQueryData(
            OPS_OVERVIEW_QUERY_KEY,
            message.payload.overview,
          )
          break
        case 'ops.activity.updated':
          if (Array.isArray(message.payload.events)) {
            queryClient.setQueryData<Array<OpsActivityEvent>>(
              opsActivityQueryKey(
                activityQueryRef.current,
                activitySeverityRef.current,
              ),
              message.payload.events,
            )
          } else if (message.payload.event != null) {
            const activityEvent = message.payload.event
            queryClient.setQueryData<Array<OpsActivityEvent>>(
              opsActivityQueryKey(
                activityQueryRef.current,
                activitySeverityRef.current,
              ),
              (previous = []) =>
                prependOpsActivityEvent(previous, activityEvent),
            )
          } else {
            void refreshActivity()
          }
          break
        default:
          break
      }
    },
    [queryClient, refreshActivity],
  )

  const connectionState = useOpsEventsSocket({
    authenticated,
    tokenRequired,
    onMessage: handleWSMessage,
  })
  const footerSummary = buildActivitiesFooterSummary({
    overviewError,
    activityError,
    overviewLoading,
    activityLoading,
    eventCount: activityEvents.length,
  })
  const footerCadence =
    activityEvents.length > 0 || activityEventsQuery.isSuccess
      ? 'Live · 5s'
      : 'waiting'

  return (
    <AppShell
      sidebar={
        <ActivitiesSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          overview={overview}
          eventCount={activityEvents.length}
          activityQuery={activityQuery}
          onActivityQueryChange={setActivityQuery}
          activitySeverity={activitySeverity}
          onActivitySeverityChange={setActivitySeverity}
          onTokenChange={setToken}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(59,130,246,.16),transparent_34%),var(--background)]">
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
            <span className="truncate text-muted-foreground">activities</span>
          </div>
          <div className="flex items-center gap-1.5">
            <TooltipHelper content="Refresh">
              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6 cursor-pointer"
                onClick={refreshPage}
                aria-label="Refresh activities"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </TooltipHelper>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden p-3">
          <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-1.5 p-2">
                {activityLoading &&
                  Array.from({ length: 5 }).map((_, idx) => (
                    <div
                      key={`activity-skeleton-${idx}`}
                      className="h-20 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                    />
                  ))}
                {activityEvents.map((event) => (
                  <div
                    key={event.id}
                    className="min-w-0 overflow-hidden rounded border border-border-subtle bg-surface-elevated px-2.5 py-2"
                  >
                    <div className="flex min-w-0 items-center justify-between gap-2">
                      <p className="min-w-0 truncate text-[12px] font-semibold">
                        {event.message}
                      </p>
                      <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        {event.severity}
                      </span>
                    </div>
                    <p className="mt-1 break-words text-[10px] text-muted-foreground">
                      {event.source} • {event.resource} •{' '}
                      {formatDateTime(event.createdAt)}
                    </p>
                    {event.details.trim() !== '' && (
                      <p className="mt-1 break-words text-[11px] text-muted-foreground">
                        {event.details}
                      </p>
                    )}
                  </div>
                ))}
                {!activityLoading && activityEvents.length === 0 && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>No activity events for the selected filters.</p>
                    <div className="flex flex-wrap gap-2">
                      {(activityQuery.trim() !== '' ||
                        activitySeverity !== 'all') && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-[11px]"
                          onClick={() => {
                            setActivityQuery('')
                            setActivitySeverity('all')
                          }}
                        >
                          Clear filters
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 text-[11px]"
                        onClick={refreshPage}
                      >
                        Refresh activities
                      </Button>
                    </div>
                  </div>
                )}
                {activityError !== '' && (
                  <div className="grid gap-2 rounded border border-dashed border-destructive/40 bg-destructive/10 p-3">
                    <p className="text-[12px] text-destructive-foreground">
                      {activityError}
                    </p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-fit text-[11px]"
                      onClick={refreshPage}
                    >
                      Try again
                    </Button>
                  </div>
                )}
              </div>
            </ScrollArea>
          </div>
        </section>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">{footerSummary}</span>
          <span className="shrink-0 whitespace-nowrap">{footerCadence}</span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/activities')({
  component: ActivitiesPage,
})
