import type { TimelineEvent } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useDateFormat } from '@/hooks/useDateFormat'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatTimelineEventLocation } from '@/lib/tmuxTimeline'
import { cn } from '@/lib/utils'

type TimelineDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  loading: boolean
  error: string
  events: Array<TimelineEvent>
  hasMore: boolean
  query: string
  severity: string
  eventType: string
  sessionFilter: string
  sessionOptions: Array<string>
  onQueryChange: (value: string) => void
  onSeverityChange: (value: string) => void
  onEventTypeChange: (value: string) => void
  onSessionFilterChange: (value: string) => void
  onRefresh: () => void
}

function severityClass(severity: string): string {
  switch (severity) {
    case 'error':
      return 'border-destructive/45 bg-destructive/15 text-destructive-foreground'
    case 'warn':
      return 'border-warning/45 bg-warning/15 text-warning-foreground'
    default:
      return 'border-border-subtle bg-surface-overlay text-muted-foreground'
  }
}

export default function TimelineDialog({
  open,
  onOpenChange,
  loading,
  error,
  events,
  hasMore,
  query,
  severity,
  eventType,
  sessionFilter,
  sessionOptions,
  onQueryChange,
  onSeverityChange,
  onEventTypeChange,
  onSessionFilterChange,
  onRefresh,
}: TimelineDialogProps) {
  const { formatDateTime } = useDateFormat()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-[5%] flex min-h-[24rem] max-h-[88vh] -translate-y-0 flex-col overflow-hidden sm:min-h-[38rem] sm:max-w-6xl">
        <DialogHeader>
          <DialogTitle>Command History</DialogTitle>
          <DialogDescription>
            Search over command lifecycle events and output markers.
          </DialogDescription>
        </DialogHeader>

        <section className="grid gap-2 rounded-md border border-border-subtle bg-secondary p-2">
          <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_9rem_11rem_12rem_auto]">
            <Input
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder="Search summary, command, cwd, marker..."
            />
            <Select value={severity} onValueChange={onSeverityChange}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">all severities</SelectItem>
                <SelectItem value="info">info</SelectItem>
                <SelectItem value="warn">warn</SelectItem>
                <SelectItem value="error">error</SelectItem>
              </SelectContent>
            </Select>
            <Select value={eventType} onValueChange={onEventTypeChange}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">all event types</SelectItem>
                <SelectItem value="command.started">command.started</SelectItem>
                <SelectItem value="command.finished">
                  command.finished
                </SelectItem>
                <SelectItem value="output.marker">output.marker</SelectItem>
              </SelectContent>
            </Select>
            <Select value={sessionFilter} onValueChange={onSessionFilterChange}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">all sessions</SelectItem>
                <SelectItem value="active">active session</SelectItem>
                {sessionOptions.map((session) => (
                  <SelectItem key={session} value={session}>
                    {session}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              type="button"
              variant="outline"
              onClick={onRefresh}
              disabled={loading}
            >
              {loading ? 'Loading...' : 'Refresh'}
            </Button>
          </div>
          <p className="text-[11px] text-muted-foreground">
            Tip: <span className="font-mono">Ctrl/Cmd + K</span> opens this
            panel.
          </p>
        </section>

        <section className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto rounded-md border border-border-subtle bg-surface-overlay p-2">
          {error.trim() !== '' && (
            <div className="mb-2 rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
              {error}
            </div>
          )}
          {events.length === 0 ? (
            <p className="py-10 text-center text-[12px] text-muted-foreground">
              {loading
                ? 'Loading timeline events...'
                : 'No events for this filter.'}
            </p>
          ) : (
            <ul className="grid gap-2">
              {events.map((event) => {
                const location = formatTimelineEventLocation(event)
                return (
                  <li
                    key={event.id}
                    className="min-w-0 rounded-md border border-border-subtle bg-secondary p-2"
                  >
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge
                        variant="outline"
                        className={severityClass(event.severity)}
                      >
                        {event.severity || 'info'}
                      </Badge>
                      <Badge variant="outline">{event.eventType}</Badge>
                      <Badge
                        variant="outline"
                        className="max-w-full truncate"
                        title={location.fallback}
                      >
                        {location.label}
                      </Badge>
                      <span className="text-[11px] text-muted-foreground">
                        {formatDateTime(event.createdAt)}
                      </span>
                      {event.durationMs > 0 && (
                        <span className="text-[11px] text-muted-foreground">
                          {event.durationMs}ms
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-[12px] font-medium">
                      {event.summary}
                    </p>
                    {(event.command || event.cwd || event.marker) && (
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {event.command && <span>cmd: {event.command}</span>}
                        {event.command && event.cwd && <span> · </span>}
                        {event.cwd && <span>cwd: {event.cwd}</span>}
                        {event.marker && <span> · marker: {event.marker}</span>}
                      </p>
                    )}
                    {event.details && (
                      <pre
                        className={cn(
                          'mt-1 max-h-28 overflow-auto whitespace-pre-wrap break-words rounded border border-border-subtle bg-card px-2 py-1 text-[11px] text-secondary-foreground',
                        )}
                      >
                        {event.details}
                      </pre>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
          {hasMore && (
            <p className="mt-2 text-[11px] text-muted-foreground">
              More events are available. Refine filters to narrow results.
            </p>
          )}
        </section>
      </DialogContent>
    </Dialog>
  )
}
