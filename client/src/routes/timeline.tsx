import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Menu, RefreshCw } from 'lucide-react'
import type {
  OpsOverviewResponse,
  OpsTimelineEvent,
  OpsTimelineResponse,
  OpsWsMessage,
} from '@/types'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import TimelineSidebar from '@/components/TimelineSidebar'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsSocket } from '@/hooks/useOpsEventsSocket'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_OVERVIEW_QUERY_KEY,
  opsTimelineQueryKey,
  prependOpsTimelineEvent,
} from '@/lib/opsQueryCache'
import { toErrorMessage } from '@/lib/opsUtils'

type TimelineFooterSummaryParams = {
  overviewError: string
  timelineError: string
  overviewLoading: boolean
  timelineLoading: boolean
  eventCount: number
}

function buildTimelineFooterSummary({
  overviewError,
  timelineError,
  overviewLoading,
  timelineLoading,
  eventCount,
}: TimelineFooterSummaryParams): string {
  if (overviewError.trim() !== '') {
    return overviewError
  }
  if (timelineError.trim() !== '') {
    return timelineError
  }
  if (overviewLoading || timelineLoading) {
    return 'Loading timeline...'
  }
  return `${eventCount} events`
}

function TimelinePage() {
  const { tokenRequired } = useMetaContext()
  const { token, setToken } = useTokenContext()
  const layout = useLayoutContext()
  const api = useTmuxApi(token)
  const queryClient = useQueryClient()

  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')

  const timelineQueryKey = useMemo(
    () => opsTimelineQueryKey(timelineQuery, timelineSeverity),
    [timelineQuery, timelineSeverity],
  )
  const timelineQueryRef = useRef(timelineQuery)
  const timelineSeverityRef = useRef(timelineSeverity)
  useEffect(() => {
    timelineQueryRef.current = timelineQuery
  }, [timelineQuery])
  useEffect(() => {
    timelineSeverityRef.current = timelineSeverity
  }, [timelineSeverity])

  const overviewQuery = useQuery({
    queryKey: OPS_OVERVIEW_QUERY_KEY,
    queryFn: async () => {
      const data = await api<OpsOverviewResponse>('/api/ops/overview')
      return data.overview
    },
  })

  const timelineEventsQuery = useQuery({
    queryKey: timelineQueryKey,
    queryFn: async () => {
      const params = new URLSearchParams({ limit: '200' })
      if (timelineQuery.trim() !== '') params.set('q', timelineQuery.trim())
      if (timelineSeverity !== 'all') params.set('severity', timelineSeverity)
      const data = await api<OpsTimelineResponse>(
        `/api/ops/timeline?${params.toString()}`,
      )
      return data.events
    },
  })

  const overview = overviewQuery.data ?? null
  const timelineEvents = timelineEventsQuery.data ?? []
  const overviewLoading = overviewQuery.isLoading
  const timelineLoading = timelineEventsQuery.isLoading
  const overviewError =
    overviewQuery.error != null
      ? toErrorMessage(overviewQuery.error, 'failed to load overview')
      : ''
  const timelineError =
    timelineEventsQuery.error != null
      ? toErrorMessage(timelineEventsQuery.error, 'failed to load timeline')
      : ''

  const refreshOverview = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: OPS_OVERVIEW_QUERY_KEY,
      exact: true,
    })
  }, [queryClient])

  const refreshTimeline = useCallback(async () => {
    await queryClient.refetchQueries({
      queryKey: opsTimelineQueryKey(
        timelineQueryRef.current,
        timelineSeverityRef.current,
      ),
      exact: true,
    })
  }, [queryClient])

  const refreshPage = useCallback(() => {
    void refreshOverview()
    void refreshTimeline()
  }, [refreshOverview, refreshTimeline])

  const handleWSMessage = useCallback(
    (message: unknown) => {
      const msg = message as OpsWsMessage
      switch (msg.type) {
        case 'ops.overview.updated':
          if (
            msg.payload.overview != null &&
            typeof msg.payload.overview === 'object'
          ) {
            queryClient.setQueryData(
              OPS_OVERVIEW_QUERY_KEY,
              msg.payload.overview,
            )
          } else {
            void refreshOverview()
          }
          break
        case 'ops.timeline.updated':
          if (Array.isArray(msg.payload.events)) {
            queryClient.setQueryData<Array<OpsTimelineEvent>>(
              opsTimelineQueryKey(
                timelineQueryRef.current,
                timelineSeverityRef.current,
              ),
              msg.payload.events,
            )
          } else if (msg.payload.event != null) {
            const timelineEvent = msg.payload.event
            queryClient.setQueryData<Array<OpsTimelineEvent>>(
              opsTimelineQueryKey(
                timelineQueryRef.current,
                timelineSeverityRef.current,
              ),
              (previous = []) =>
                prependOpsTimelineEvent(previous, timelineEvent),
            )
          } else {
            void refreshTimeline()
          }
          break
        default:
          break
      }
    },
    [queryClient, refreshOverview, refreshTimeline],
  )

  const connectionState = useOpsEventsSocket({
    token,
    tokenRequired,
    onMessage: handleWSMessage,
  })
  const footerSummary = buildTimelineFooterSummary({
    overviewError,
    timelineError,
    overviewLoading,
    timelineLoading,
    eventCount: timelineEvents.length,
  })
  const footerCadence =
    timelineEvents.length > 0 || timelineEventsQuery.isSuccess
      ? 'Live · 5s'
      : 'waiting'

  return (
    <AppShell
      sidebar={
        <TimelineSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          token={token}
          overview={overview}
          eventCount={timelineEvents.length}
          timelineQuery={timelineQuery}
          onTimelineQueryChange={setTimelineQuery}
          timelineSeverity={timelineSeverity}
          onTimelineSeverityChange={setTimelineSeverity}
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
            <span className="truncate text-muted-foreground">timeline</span>
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh timeline"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
            <ConnectionBadge state={connectionState} />
          </div>
        </header>

        <section className="grid min-h-0 grid-rows-[1fr] overflow-hidden p-3">
          <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-1.5 p-2">
                {timelineLoading &&
                  Array.from({ length: 5 }).map((_, idx) => (
                    <div
                      key={`timeline-skeleton-${idx}`}
                      className="h-20 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                    />
                  ))}
                {timelineEvents.map((event) => (
                  <div
                    key={event.id}
                    className="rounded border border-border-subtle bg-surface-elevated px-2.5 py-2"
                  >
                    <div className="flex min-w-0 items-center justify-between gap-2">
                      <p className="min-w-0 truncate text-[12px] font-semibold">
                        {event.message}
                      </p>
                      <span className="shrink-0 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        {event.severity}
                      </span>
                    </div>
                    <p className="mt-1 text-[10px] text-muted-foreground">
                      {event.source} • {event.resource} • {event.createdAt}
                    </p>
                    {event.details.trim() !== '' && (
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {event.details}
                      </p>
                    )}
                  </div>
                ))}
                {!timelineLoading && timelineEvents.length === 0 && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>No timeline events for the selected filters.</p>
                    <div className="flex flex-wrap gap-2">
                      {(timelineQuery.trim() !== '' ||
                        timelineSeverity !== 'all') && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-[11px]"
                          onClick={() => {
                            setTimelineQuery('')
                            setTimelineSeverity('all')
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
                        Refresh timeline
                      </Button>
                    </div>
                  </div>
                )}
                {timelineError !== '' && (
                  <div className="grid gap-2 rounded border border-dashed border-destructive/40 bg-destructive/10 p-3">
                    <p className="text-[12px] text-destructive-foreground">
                      {timelineError}
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

export const Route = createFileRoute('/timeline')({
  component: TimelinePage,
})
