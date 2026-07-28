import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Database, ShieldCheck } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import SettingsSectionHeader from './SettingsSectionHeader'
import type { StorageStatsResponse } from '@/types'
import { Button } from '@/components/ui/button'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { OPS_STORAGE_STATS_QUERY_KEY } from '@/lib/opsQueryCache'
import { formatBytes } from '@/lib/opsUtils'

export default function StorageSettings() {
  const api = useTmuxApi()
  const statsQuery = useQuery({
    queryKey: OPS_STORAGE_STATS_QUERY_KEY,
    queryFn: () => api<StorageStatsResponse>('/api/ops/storage/stats'),
  })
  const stats = statsQuery.data ?? null
  const totalRows = stats?.resources.reduce((sum, resource) => sum + resource.totalRows, 0) ?? 0
  const protectedRows =
    stats?.resources.reduce((sum, resource) => sum + resource.protectedRows, 0) ?? 0
  const flushableRows =
    stats?.resources.reduce((sum, resource) => sum + resource.flushableRows, 0) ?? 0

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Storage"
        description="Review persisted operational history here. Destructive actions stay in a dedicated maintenance workspace."
        icon={<Database className="size-4" aria-hidden="true" />}
      />

      {statsQuery.isLoading && (
        <div
          aria-label="Loading storage overview"
          className="grid grid-cols-2 gap-2 sm:grid-cols-4"
        >
          {Array.from({ length: 4 }, (_, index) => (
            <span
              key={index}
              className="h-20 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-card"
            />
          ))}
        </div>
      )}

      {statsQuery.isError && stats == null && (
        <div
          role="alert"
          className="grid gap-3 rounded-lg border border-destructive/45 bg-card p-3"
        >
          <p className="text-[11px] text-destructive-foreground">
            {statsQuery.error instanceof Error
              ? statsQuery.error.message
              : 'Storage overview is unavailable.'}
          </p>
          <Button
            variant="outline"
            className="min-h-11 w-full sm:w-fit"
            onClick={() => void statsQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      )}

      {stats && (
        <>
          <section aria-label="Storage summary" className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <StorageMetric label="Disk footprint" value={formatBytes(stats.totalBytes)} />
            <StorageMetric label="History rows" value={String(totalRows)} />
            <StorageMetric label="Eligible" value={String(flushableRows)} />
            <StorageMetric label="Protected" value={String(protectedRows)} tone="protected" />
          </section>

          {stats.resources.length === 0 && (
            <p className="rounded-lg border border-dashed border-border p-4 text-[11px] text-muted-foreground">
              No persisted history resources were reported.
            </p>
          )}

          <section className="grid gap-3 rounded-lg border border-border-subtle bg-card p-3 sm:p-4">
            <div className="flex items-start gap-2">
              <ShieldCheck className="mt-0.5 size-4 shrink-0 text-ok-foreground" />
              <div>
                <h2 className="text-[12px] font-medium">Active executions stay protected</h2>
                <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
                  Queued, running, and approval-waiting jobs cannot be removed by cleanup.
                </p>
              </div>
            </div>
            <Button asChild variant="outline" className="min-h-11 w-full sm:w-fit">
              <Link to="/maintenance/storage">
                Open storage maintenance
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Link>
            </Button>
          </section>
        </>
      )}
    </div>
  )
}

function StorageMetric({
  label,
  value,
  tone = 'default',
}: {
  label: string
  value: string
  tone?: 'default' | 'protected'
}) {
  return (
    <div className="rounded-lg border border-border-subtle bg-card p-3">
      <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">{label}</p>
      <p
        className={`mt-2 font-mono text-lg font-semibold ${
          tone === 'protected' ? 'text-ok-foreground' : 'text-foreground'
        }`}
      >
        {value}
      </p>
    </div>
  )
}
