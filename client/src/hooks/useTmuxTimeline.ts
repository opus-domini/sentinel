import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { TimelineEvent, TimelineResponse } from '@/types'
import type { ApiFunction, TmuxTimelineCache } from './tmuxTypes'
import { tmuxTimelineQueryKey } from '@/lib/tmuxQueryCache'
import { buildTimelineQueryString } from '@/lib/tmuxTimeline'

type UseTmuxTimelineOptions = {
  api: ApiFunction
  activeSession: string
}

export function useTmuxTimeline(options: UseTmuxTimelineOptions) {
  const { api, activeSession } = options
  const queryClient = useQueryClient()
  const timelineGenerationRef = useRef(0)

  const [timelineOpen, setTimelineOpen] = useState(false)
  const [timelineEvents, setTimelineEvents] = useState<Array<TimelineEvent>>(
    () =>
      queryClient.getQueryData<TmuxTimelineCache>(
        tmuxTimelineQueryKey({
          session: '',
          query: '',
          severity: 'all',
          eventType: 'all',
          limit: 180,
        }),
      )?.events ?? [],
  )
  const [timelineHasMore, setTimelineHasMore] = useState(
    () =>
      queryClient.getQueryData<TmuxTimelineCache>(
        tmuxTimelineQueryKey({
          session: '',
          query: '',
          severity: 'all',
          eventType: 'all',
          limit: 180,
        }),
      )?.hasMore ?? false,
  )
  const [timelineLoading, setTimelineLoading] = useState(false)
  const [timelineError, setTimelineError] = useState('')
  const [timelineQuery, setTimelineQuery] = useState('')
  const [timelineSeverity, setTimelineSeverity] = useState('all')
  const [timelineEventType, setTimelineEventType] = useState('all')
  const [timelineSessionFilter, setTimelineSessionFilter] = useState('active')

  const timelineOpenRef = useRef(false)
  const timelineSessionFilterRef = useRef('active')
  const loadTimelineRef = useRef<(options?: { quiet?: boolean }) => void>(
    () => {
      return
    },
  )

  useEffect(() => {
    timelineOpenRef.current = timelineOpen
  }, [timelineOpen])
  useEffect(() => {
    timelineSessionFilterRef.current = timelineSessionFilter
  }, [timelineSessionFilter])

  const resolveTimelineSessionScope = useCallback(
    (scope: string): string => {
      const normalized = scope.trim()
      if (normalized === '' || normalized === 'all') {
        return ''
      }
      if (normalized === 'active') {
        return activeSession.trim()
      }
      return normalized
    },
    [activeSession],
  )

  // Sync timeline cache
  useEffect(() => {
    const session = resolveTimelineSessionScope(timelineSessionFilter)
    queryClient.setQueryData<TmuxTimelineCache>(
      tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      }),
      {
        events: timelineEvents,
        hasMore: timelineHasMore,
      },
    )
  }, [
    queryClient,
    resolveTimelineSessionScope,
    timelineEventType,
    timelineEvents,
    timelineHasMore,
    timelineQuery,
    timelineSessionFilter,
    timelineSeverity,
  ])

  const loadTimeline = useCallback(
    async (params?: { quiet?: boolean }) => {
      const gen = ++timelineGenerationRef.current
      if (!params?.quiet) {
        setTimelineLoading(true)
      }
      const session = resolveTimelineSessionScope(
        timelineSessionFilterRef.current,
      )
      const cacheKey = tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      })
      const cached = queryClient.getQueryData<TmuxTimelineCache>(cacheKey)
      if (cached != null) {
        setTimelineEvents(cached.events)
        setTimelineHasMore(cached.hasMore)
      }
      const queryString = buildTimelineQueryString({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      })
      try {
        const data = await api<TimelineResponse>(
          `/api/tmux/timeline${queryString}`,
        )
        if (gen !== timelineGenerationRef.current) return
        setTimelineEvents(data.events)
        setTimelineHasMore(data.hasMore)
        queryClient.setQueryData<TmuxTimelineCache>(cacheKey, {
          events: data.events,
          hasMore: data.hasMore,
        })
        setTimelineError('')
      } catch (error) {
        if (gen !== timelineGenerationRef.current) return
        const message =
          error instanceof Error ? error.message : 'failed to load timeline'
        setTimelineError(message)
      } finally {
        if (gen === timelineGenerationRef.current) {
          setTimelineLoading(false)
        }
      }
    },
    [
      api,
      queryClient,
      resolveTimelineSessionScope,
      timelineEventType,
      timelineQuery,
      timelineSeverity,
    ],
  )

  useEffect(() => {
    loadTimelineRef.current = (params?: { quiet?: boolean }) => {
      void loadTimeline(params)
    }
  }, [loadTimeline])

  // Auto-load timeline when dialog opens or filters change
  useEffect(() => {
    if (!timelineOpen) {
      return
    }
    const session = resolveTimelineSessionScope(timelineSessionFilter)
    const cached = queryClient.getQueryData<TmuxTimelineCache>(
      tmuxTimelineQueryKey({
        session,
        query: timelineQuery,
        severity: timelineSeverity,
        eventType: timelineEventType,
        limit: 180,
      }),
    )
    if (cached != null) {
      setTimelineEvents(cached.events)
      setTimelineHasMore(cached.hasMore)
      setTimelineError('')
      setTimelineLoading(false)
    }
    const timeoutID = window.setTimeout(() => {
      void loadTimeline()
    }, 120)
    return () => {
      window.clearTimeout(timeoutID)
    }
  }, [
    loadTimeline,
    queryClient,
    resolveTimelineSessionScope,
    timelineOpen,
    timelineQuery,
    timelineSeverity,
    timelineEventType,
    timelineSessionFilter,
    activeSession,
  ])

  // Keyboard shortcut Ctrl+K / Cmd+K
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey)) {
        return
      }
      if (event.key.toLowerCase() !== 'k') {
        return
      }
      event.preventDefault()
      setTimelineOpen(true)
      void loadTimeline({ quiet: true })
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [loadTimeline])

  return {
    // State
    timelineOpen,
    timelineEvents,
    timelineHasMore,
    timelineLoading,
    timelineError,
    timelineQuery,
    timelineSeverity,
    timelineEventType,
    timelineSessionFilter,
    // Refs (needed by events socket)
    timelineOpenRef,
    timelineSessionFilterRef,
    loadTimelineRef,
    // Actions
    setTimelineOpen,
    setTimelineQuery,
    setTimelineSeverity,
    setTimelineEventType,
    setTimelineSessionFilter,
    loadTimeline,
    resolveTimelineSessionScope,
  }
}
