import * as React from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { LogLevel, ParsedLogLine } from '@/lib/log-parser'
import { cn } from '@/lib/utils'

interface LogViewerProps {
  lines: Array<ParsedLogLine>
  loading: boolean
  searchQuery: string
  wordWrap: boolean
  follow: boolean
  onFollowChange: (follow: boolean) => void
  className?: string
}

const levelColors: Record<LogLevel, string> = {
  error: 'text-red-400',
  warn: 'text-yellow-400',
  info: 'text-log-info',
  debug: 'text-neutral-500',
  notice: 'text-log-notice',
  unknown: 'text-secondary-foreground',
}

const ESTIMATED_ROW_HEIGHT = 16
const VIRTUAL_OVERSCAN_ROWS = 24

function virtualRows(
  lines: Array<ParsedLogLine>,
  heights: ReadonlyMap<number, number>,
  scrollTop: number,
  viewportHeight: number,
) {
  const offsets = new Array<number>(lines.length)
  let totalHeight = 0
  for (let index = 0; index < lines.length; index += 1) {
    offsets[index] = totalHeight
    totalHeight += heights.get(lines[index].lineNumber) ?? ESTIMATED_ROW_HEIGHT
  }

  const overscan = ESTIMATED_ROW_HEIGHT * VIRTUAL_OVERSCAN_ROWS
  const startTarget = Math.max(0, scrollTop - overscan)
  const endTarget = scrollTop + Math.max(viewportHeight, ESTIMATED_ROW_HEIGHT) + overscan
  let startIndex = 0
  while (
    startIndex < lines.length &&
    offsets[startIndex] + (heights.get(lines[startIndex].lineNumber) ?? ESTIMATED_ROW_HEIGHT) <
      startTarget
  ) {
    startIndex += 1
  }
  let endIndex = startIndex
  while (endIndex < lines.length && offsets[endIndex] < endTarget) {
    endIndex += 1
  }

  return {
    totalHeight,
    rows: lines.slice(startIndex, endIndex).map((line, offset) => ({
      index: startIndex + offset,
      line,
      start: offsets[startIndex + offset],
    })),
  }
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function highlightMatch(text: string, regex: RegExp | null): React.ReactNode {
  if (!regex) return text
  const parts = text.split(regex)
  if (parts.length === 1) return text
  let offset = 0
  return parts.map((part, i) => {
    const start = offset
    offset += part.length

    return i % 2 === 1 ? (
      <mark key={`${start}:${part}`} className="rounded-sm bg-yellow-500/30 text-yellow-200">
        {part}
      </mark>
    ) : (
      part
    )
  })
}

const LogLine = React.memo(function LogLine({
  line,
  gutterWidth,
  searchRegex,
  wordWrap,
}: {
  line: ParsedLogLine
  gutterWidth: number
  searchRegex: RegExp | null
  wordWrap: boolean
}) {
  const levelClass = levelColors[line.level]

  return (
    <div className="flex hover:bg-surface-hover">
      <span
        className="shrink-0 select-none whitespace-nowrap pr-3 text-right text-muted-foreground/60"
        style={{ width: `${gutterWidth}ch` }}
      >
        {line.lineNumber}
      </span>
      <span
        className={cn(
          'min-w-0 flex-1',
          wordWrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre',
        )}
      >
        {line.timestamp && (
          <span className="text-log-timestamp/70">
            {highlightMatch(line.timestamp, searchRegex)}
          </span>
        )}
        {line.timestamp && ' '}
        {line.unit && (
          <span className="text-purple-400/70">{highlightMatch(line.unit, searchRegex)}</span>
        )}
        {line.unit && ' '}
        <span className={levelClass}>{highlightMatch(line.message, searchRegex)}</span>
      </span>
    </div>
  )
})

export function LogViewer({
  lines,
  loading,
  searchQuery,
  wordWrap,
  follow,
  onFollowChange,
  className,
}: LogViewerProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const isUserScrolling = useRef(false)
  const [viewport, setViewport] = useState({ scrollTop: 0, height: 0 })
  const [rowHeights, setRowHeights] = useState<ReadonlyMap<number, number>>(() => new Map())
  const [measuredWordWrap, setMeasuredWordWrap] = useState(wordWrap)

  if (wordWrap !== measuredWordWrap) {
    setMeasuredWordWrap(wordWrap)
    setRowHeights(new Map())
  }

  const gutterWidth = useMemo(() => {
    if (lines.length === 0) return 3
    return Math.max(3, String(lines[lines.length - 1].lineNumber).length + 1)
  }, [lines])

  const searchRegex = useMemo(
    () => (searchQuery ? new RegExp(`(${escapeRegex(searchQuery)})`, 'i') : null),
    [searchQuery],
  )

  const filteredLines = useMemo(() => {
    if (!searchQuery) return lines
    const q = searchQuery.toLowerCase()
    return lines.filter((l) => l.raw.toLowerCase().includes(q))
  }, [lines, searchQuery])

  const virtualized = useMemo(
    () => virtualRows(filteredLines, rowHeights, viewport.scrollTop, viewport.height),
    [filteredLines, rowHeights, viewport.height, viewport.scrollTop],
  )

  const measureRow = useCallback((lineNumber: number, element: HTMLDivElement | null) => {
    if (element === null) return
    const height = Math.max(1, Math.ceil(element.getBoundingClientRect().height))
    setRowHeights((current) => {
      if (current.get(lineNumber) === height) return current
      const next = new Map(current)
      next.set(lineNumber, height)
      return next
    })
  }, [])

  useEffect(() => {
    const element = scrollRef.current
    if (element === null || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => {
      setViewport({ scrollTop: element.scrollTop, height: element.clientHeight })
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  // Auto-scroll to bottom when follow is enabled and new lines arrive
  useEffect(() => {
    if (follow && scrollRef.current && !isUserScrolling.current && filteredLines.length > 0) {
      scrollRef.current.scrollTop = Math.max(
        0,
        virtualized.totalHeight - scrollRef.current.clientHeight,
      )
    }
  }, [follow, filteredLines.length, virtualized.totalHeight])

  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    setViewport({ scrollTop: el.scrollTop, height: el.clientHeight })
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 24
    if (!atBottom && follow) {
      isUserScrolling.current = true
      onFollowChange(false)
      // Reset flag after a tick
      requestAnimationFrame(() => {
        isUserScrolling.current = false
      })
    } else if (atBottom && !follow) {
      onFollowChange(true)
    }
  }, [follow, onFollowChange])

  if (loading) {
    return (
      <div className={cn('flex items-center justify-center p-4', className)}>
        <p className="text-[12px] text-muted-foreground">Loading logs...</p>
      </div>
    )
  }

  if (lines.length === 0) {
    return (
      <div className={cn('flex items-center justify-center p-4', className)}>
        <p className="text-[12px] text-muted-foreground">No logs available.</p>
      </div>
    )
  }

  return (
    <div
      ref={scrollRef}
      onScroll={handleScroll}
      className={cn(
        'overflow-auto overscroll-contain rounded border border-border-subtle bg-background',
        wordWrap ? 'overflow-x-hidden' : '',
        className,
      )}
    >
      <div
        className="relative font-mono text-[11px]"
        role="log"
        style={{ height: virtualized.totalHeight }}
      >
        {virtualized.rows.map((virtualRow) => {
          const line = virtualRow.line
          return (
            <div
              key={line.lineNumber}
              data-index={virtualRow.index}
              ref={(element) => measureRow(line.lineNumber, element)}
              className="absolute left-0 top-0 w-full px-2"
              style={{ transform: `translateY(${virtualRow.start}px)` }}
            >
              <LogLine
                line={line}
                gutterWidth={gutterWidth}
                searchRegex={searchRegex}
                wordWrap={wordWrap}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
