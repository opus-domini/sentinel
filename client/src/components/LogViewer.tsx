import * as React from 'react'
import { useCallback, useEffect, useMemo, useRef } from 'react'

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
  info: 'text-blue-300',
  debug: 'text-neutral-500',
  notice: 'text-cyan-400',
  unknown: 'text-secondary-foreground',
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function highlightMatch(text: string, query: string): React.ReactNode {
  if (!query) return text
  const regex = new RegExp(`(${escapeRegex(query)})`, 'gi')
  const parts = text.split(regex)
  if (parts.length === 1) return text
  return parts.map((part, i) =>
    regex.test(part) ? (
      <mark key={i} className="rounded-sm bg-yellow-500/30 text-yellow-200">
        {part}
      </mark>
    ) : (
      part
    ),
  )
}

const LogLine = React.memo(function LogLine({
  line,
  gutterWidth,
  searchQuery,
  wordWrap,
}: {
  line: ParsedLogLine
  gutterWidth: number
  searchQuery: string
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
          <span className="text-blue-400/70">
            {highlightMatch(line.timestamp, searchQuery)}
          </span>
        )}
        {line.timestamp && ' '}
        {line.unit && (
          <span className="text-purple-400/70">
            {highlightMatch(line.unit, searchQuery)}
          </span>
        )}
        {line.unit && ' '}
        <span className={levelClass}>
          {highlightMatch(line.message, searchQuery)}
        </span>
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

  const gutterWidth = useMemo(() => {
    if (lines.length === 0) return 3
    return Math.max(3, String(lines[lines.length - 1].lineNumber).length + 1)
  }, [lines])

  const filteredLines = useMemo(() => {
    if (!searchQuery) return lines
    const q = searchQuery.toLowerCase()
    return lines.filter((l) => l.raw.toLowerCase().includes(q))
  }, [lines, searchQuery])

  // Auto-scroll to bottom when follow is enabled and new lines arrive
  useEffect(() => {
    if (follow && scrollRef.current && !isUserScrolling.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [follow, filteredLines.length])

  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
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
        'overflow-auto rounded border border-border-subtle bg-background',
        wordWrap ? 'overflow-x-hidden' : '',
        className,
      )}
    >
      <div className="p-2 font-mono text-[11px]">
        {filteredLines.map((line) => (
          <LogLine
            key={line.lineNumber}
            line={line}
            gutterWidth={gutterWidth}
            searchQuery={searchQuery}
            wordWrap={wordWrap}
          />
        ))}
      </div>
    </div>
  )
}
