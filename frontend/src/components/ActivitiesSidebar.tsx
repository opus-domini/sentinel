import { useMemo, useState } from 'react'
import { Lock, LockOpen } from 'lucide-react'
import type { OpsOverview } from '@/types'
import { ACTIVITY_SOURCES } from '@/lib/activityIcons'
import SidebarShell from '@/components/sidebar/SidebarShell'
import ActivitiesHelpDialog from '@/components/ActivitiesHelpDialog'
import TokenDialog from '@/components/sidebar/TokenDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TooltipHelper } from '@/components/TooltipHelper'
import { formatUptime } from '@/lib/opsUtils'

type ActivitiesSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  overview: OpsOverview | null
  eventCount: number
  activityQuery: string
  onActivityQueryChange: (value: string) => void
  activitySeverity: string
  onActivitySeverityChange: (value: string) => void
  activitySource: string
  onActivitySourceChange: (value: string) => void
  onTokenChange: (value: string) => void
}

export default function ActivitiesSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  authenticated,
  overview,
  eventCount,
  activityQuery,
  onActivityQueryChange,
  activitySeverity,
  onActivitySeverityChange,
  activitySource,
  onActivitySourceChange,
  onTokenChange,
}: ActivitiesSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return authenticated ? 'Authenticated (required)' : 'Token required'
    }
    return authenticated ? 'Authenticated' : 'Authentication optional'
  }, [authenticated, tokenRequired])

  const health = useMemo(() => {
    if (overview == null) return '-'
    return overview.services.failed > 0
      ? `${overview.services.failed} failed`
      : 'healthy'
  }, [overview])

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
              Activities
            </span>
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full border border-border px-1.5 text-[11px] text-secondary-foreground">
              {eventCount}
            </span>
            <div className="ml-auto flex items-center gap-1">
              <ActivitiesHelpDialog />
              <TooltipHelper content={lockLabel}>
                <Button
                  variant="outline"
                  size="icon-xs"
                  className="cursor-pointer text-secondary-foreground"
                  onClick={() => setIsTokenOpen(true)}
                  aria-label="API token"
                >
                  {authenticated ? (
                    <Lock className="h-3 w-3" />
                  ) : (
                    <LockOpen className="h-3 w-3" />
                  )}
                </Button>
              </TooltipHelper>
            </div>
          </div>

          <TokenDialog
            open={isTokenOpen}
            onOpenChange={setIsTokenOpen}
            authenticated={authenticated}
            onTokenChange={onTokenChange}
            tokenRequired={tokenRequired}
          />
        </section>

        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Overview
          </span>
          <div className="grid gap-1.5">
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Host</p>
              <p className="mt-0.5 min-w-0 truncate text-[11px] font-medium">
                {overview != null
                  ? `${overview.host.hostname || '-'} (${overview.host.os}/${overview.host.arch})`
                  : '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Uptime</p>
              <p className="mt-0.5 text-[11px] font-medium">
                {overview != null
                  ? formatUptime(overview.sentinel.uptimeSec)
                  : '-'}
              </p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Events</p>
              <p className="mt-0.5 text-[11px] font-medium">{eventCount}</p>
            </div>
            <div className="rounded-md border border-border-subtle bg-surface-elevated p-2">
              <p className="text-[10px] text-muted-foreground">Health</p>
              <p className="mt-0.5 text-[11px] font-medium">{health}</p>
            </div>
          </div>
        </section>

        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            Filters
          </span>
          <Input
            name="activities-search"
            value={activityQuery}
            onChange={(e) => onActivityQueryChange(e.target.value)}
            placeholder="search activities"
            className="h-7 bg-surface-overlay text-[12px] md:h-8"
          />
          <Select
            value={activitySeverity}
            onValueChange={onActivitySeverityChange}
          >
            <SelectTrigger className="h-7 w-full cursor-pointer bg-surface-overlay text-[12px] md:h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all" className="cursor-pointer">
                all severities
              </SelectItem>
              <SelectItem value="info" className="cursor-pointer">
                info
              </SelectItem>
              <SelectItem value="warn" className="cursor-pointer">
                warn
              </SelectItem>
              <SelectItem value="error" className="cursor-pointer">
                error
              </SelectItem>
            </SelectContent>
          </Select>
          <Select value={activitySource} onValueChange={onActivitySourceChange}>
            <SelectTrigger className="h-7 w-full cursor-pointer bg-surface-overlay text-[12px] md:h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all" className="cursor-pointer">
                all sources
              </SelectItem>
              {ACTIVITY_SOURCES.map((source) => (
                <SelectItem
                  key={source}
                  value={source}
                  className="cursor-pointer"
                >
                  {source}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </section>
      </div>
    </SidebarShell>
  )
}
