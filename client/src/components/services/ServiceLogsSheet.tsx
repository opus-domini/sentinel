import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowDownToLine,
  Radio,
  RefreshCw,
  Search,
  WrapText,
  X,
} from 'lucide-react'
import type {
  OpsBrowsedService,
  OpsServiceLogsResponse,
  OpsUnitLogsResponse,
} from '@/types'
import type { ParsedLogLine } from '@/lib/log-parser'
import { LogViewer } from '@/components/LogViewer'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import { useLogStream } from '@/hooks/useLogStream'
import { parseLogLines, parseSingleLine } from '@/lib/log-parser'
import { cn } from '@/lib/utils'

const LOG_BUFFER_MAX = 5_000

type ServiceLogsSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Monotonically increasing counter; bump to re-fetch initial logs. */
  fetchKey: number
  service: OpsBrowsedService | null
  authenticated: boolean
  tokenRequired: boolean
  api: <T>(url: string, init?: RequestInit) => Promise<T>
}

export function ServiceLogsSheet({
  open,
  onOpenChange,
  fetchKey,
  service,
  authenticated,
  tokenRequired,
  api,
}: ServiceLogsSheetProps) {
  const [logLines, setLogLines] = useState<Array<ParsedLogLine>>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebouncedValue(search)
  const [wrap, setWrap] = useState(false)
  const [follow, setFollow] = useState(true)
  const [streamEnabled, setStreamEnabled] = useState(false)
  const lineCounterRef = useRef(0)
  const serviceRef = useRef<OpsBrowsedService | null>(null)

  // Fetch initial logs when fetchKey changes (i.e. when opened for a service)
  useEffect(() => {
    if (!service || !open) return
    serviceRef.current = service
    setLoading(true)
    setLogLines([])
    setSearch('')
    setFollow(true)
    setStreamEnabled(false)
    lineCounterRef.current = 0

    let cancelled = false
    const fetchLogs = async () => {
      try {
        let output = ''
        if (service.tracked && service.trackedName) {
          const data = await api<OpsServiceLogsResponse>(
            `/api/ops/services/${encodeURIComponent(service.trackedName)}/logs?lines=200`,
          )
          output = data.output
        } else {
          const params = new URLSearchParams({
            unit: service.unit,
            scope: service.scope,
            manager: service.manager,
            lines: '200',
          })
          const data = await api<OpsUnitLogsResponse>(
            `/api/ops/services/unit/logs?${params.toString()}`,
          )
          output = data.output
        }
        if (cancelled) return
        const parsed = parseLogLines(output)
        lineCounterRef.current = parsed.length
        setLogLines(parsed)
        setStreamEnabled(true)
      } catch {
        if (cancelled) return
        setLogLines([
          {
            lineNumber: 1,
            raw: '(failed to fetch logs)',
            timestamp: '',
            hostname: '',
            unit: '',
            message: '(failed to fetch logs)',
            level: 'error',
          },
        ])
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void fetchLogs()
    return () => {
      cancelled = true
    }
    // fetchKey drives re-fetching; service/open are also checked but fetchKey
    // is what changes each time the user clicks "Logs" on a row.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchKey])

  const handleStreamLine = useCallback((line: string) => {
    lineCounterRef.current += 1
    const parsed = parseSingleLine(line, lineCounterRef.current)
    setLogLines((prev) => {
      const next = [...prev, parsed]
      if (next.length > LOG_BUFFER_MAX) {
        return next.slice(next.length - LOG_BUFFER_MAX)
      }
      return next
    })
  }, [])

  const streamTarget = useMemo(() => {
    if (!service || !open) return null
    if (service.tracked && service.trackedName) {
      return { kind: 'service' as const, name: service.trackedName }
    }
    return {
      kind: 'unit' as const,
      unit: service.unit,
      scope: service.scope,
      manager: service.manager,
    }
  }, [open, service])

  const streamStatus = useLogStream({
    authenticated,
    tokenRequired,
    target: streamTarget,
    enabled: streamEnabled && open,
    onLine: handleStreamLine,
  })

  const refreshLogs = useCallback(async () => {
    const svc = serviceRef.current
    if (!svc) return
    setStreamEnabled(false)
    try {
      let output = ''
      if (svc.tracked && svc.trackedName) {
        const data = await api<OpsServiceLogsResponse>(
          `/api/ops/services/${encodeURIComponent(svc.trackedName)}/logs?lines=200`,
        )
        output = data.output
      } else {
        const params = new URLSearchParams({
          unit: svc.unit,
          scope: svc.scope,
          manager: svc.manager,
          lines: '200',
        })
        const data = await api<OpsUnitLogsResponse>(
          `/api/ops/services/unit/logs?${params.toString()}`,
        )
        output = data.output
      }
      const parsed = parseLogLines(output)
      lineCounterRef.current = parsed.length
      setLogLines(parsed)
      setFollow(true)
      setStreamEnabled(true)
    } catch {
      // keep existing lines on refresh failure
    }
  }, [api])

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      onOpenChange(nextOpen)
      if (!nextOpen) {
        setStreamEnabled(false)
        serviceRef.current = null
      }
    },
    [onOpenChange],
  )

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent className="flex flex-col gap-0 p-0">
        <SheetHeader className="shrink-0 border-b border-border-subtle px-4 py-3">
          <SheetTitle>{service?.unit || 'Service logs'}</SheetTitle>
          <SheetDescription>
            {streamEnabled && streamStatus === 'connected' ? (
              <span className="inline-flex items-center gap-1.5">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
                </span>
                Streaming live
              </span>
            ) : (
              'Recent log output'
            )}
          </SheetDescription>
        </SheetHeader>

        <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border-subtle px-4 py-2">
          <div className="relative min-w-0 flex-1">
            <Search className="absolute left-2 top-1.5 h-3.5 w-3.5 text-muted-foreground" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search logs..."
              className={cn(
                'h-7 w-full rounded-md border border-border-subtle bg-surface-overlay pl-7 text-[11px] placeholder:text-muted-foreground',
                search ? 'pr-7' : 'pr-2',
              )}
            />
            {search && (
              <button
                type="button"
                className="absolute right-1.5 top-1 inline-flex h-5 w-5 cursor-pointer items-center justify-center rounded text-muted-foreground hover:text-foreground"
                onClick={() => setSearch('')}
                aria-label="Clear search"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
          <TooltipHelper content="Word wrap">
            <Button
              variant={wrap ? 'default' : 'outline'}
              size="sm"
              className="h-7 w-7 cursor-pointer p-0"
              onClick={() => setWrap((v) => !v)}
              aria-label="Toggle word wrap"
            >
              <WrapText className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Refresh logs">
            <Button
              variant="outline"
              size="sm"
              className="h-7 w-7 cursor-pointer p-0"
              onClick={refreshLogs}
              aria-label="Refresh logs"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <TooltipHelper
            content={streamEnabled ? 'Stop streaming' : 'Stream live'}
          >
            <Button
              variant={streamEnabled ? 'default' : 'outline'}
              size="sm"
              className={cn(
                'h-7 w-7 cursor-pointer p-0',
                streamEnabled && streamStatus === 'connected'
                  ? 'text-emerald-400'
                  : '',
              )}
              onClick={() => setStreamEnabled((v) => !v)}
              aria-label={streamEnabled ? 'Stop streaming' : 'Stream live'}
            >
              <Radio className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
          <TooltipHelper content="Follow output">
            <Button
              variant={follow ? 'default' : 'outline'}
              size="sm"
              className="h-7 w-7 cursor-pointer p-0"
              onClick={() => setFollow((v) => !v)}
              aria-label="Toggle follow"
            >
              <ArrowDownToLine className="h-3.5 w-3.5" />
            </Button>
          </TooltipHelper>
        </div>

        <LogViewer
          lines={logLines}
          loading={loading}
          searchQuery={debouncedSearch}
          wordWrap={wrap}
          follow={follow}
          onFollowChange={setFollow}
          className="min-h-0 flex-1 rounded-none border-0"
        />
      </SheetContent>
    </Sheet>
  )
}
