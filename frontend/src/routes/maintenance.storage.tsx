import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { ArrowLeft, Database, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react'

import type { StorageFlushResponse, StorageResourceStat, StorageStatsResponse } from '@/types'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { OPS_STORAGE_STATS_QUERY_KEY } from '@/lib/opsQueryCache'
import { formatBytes } from '@/lib/opsUtils'

const allResources = 'all'

export const Route = createFileRoute('/maintenance/storage')({
  component: StorageMaintenanceRoute,
})

function StorageMaintenanceRoute() {
  return (
    <AppShell>
      <StorageMaintenancePage />
    </AppShell>
  )
}

export function StorageMaintenancePage() {
  const { hostname } = useMetaContext()
  const api = useTmuxApi()
  const [selectedResource, setSelectedResource] = useState(allResources)
  const [confirmResource, setConfirmResource] = useState<string | null>(null)
  const [flushingResource, setFlushingResource] = useState('')
  const [flushError, setFlushError] = useState('')
  const [notice, setNotice] = useState('')

  const statsQuery = useQuery({
    queryKey: OPS_STORAGE_STATS_QUERY_KEY,
    queryFn: () => api<StorageStatsResponse>('/api/ops/storage/stats'),
  })
  const stats = statsQuery.data ?? null
  const selectedImpact = useMemo(
    () => storageImpact(stats?.resources ?? [], selectedResource),
    [selectedResource, stats?.resources],
  )
  const confirmImpact = useMemo(
    () => storageImpact(stats?.resources ?? [], confirmResource ?? allResources),
    [confirmResource, stats?.resources],
  )
  const totalFlushable = sumRows(stats?.resources ?? [], 'flushableRows')

  const executeFlush = async (resource: string) => {
    setFlushingResource(resource)
    setFlushError('')
    setNotice('')
    try {
      const response = await api<StorageFlushResponse>('/api/ops/storage/flush', {
        method: 'POST',
        body: JSON.stringify({ resource }),
      })
      const removedRows = response.results.reduce((total, item) => total + item.removedRows, 0)
      const protectedRows = response.results.reduce((total, item) => total + item.protectedRows, 0)
      setNotice(
        `${formatRows(removedRows)} eligible row${removedRows === 1 ? '' : 's'} removed. ` +
          `${formatRows(protectedRows)} active row${protectedRows === 1 ? '' : 's'} preserved.`,
      )
      await statsQuery.refetch()
    } catch (error) {
      setFlushError(error instanceof Error ? error.message : 'Storage cleanup failed')
    } finally {
      setFlushingResource('')
    }
  }

  return (
    <main className="grid h-full min-h-0 min-w-0 grid-rows-[40px_1fr] bg-[radial-gradient(circle_at_18%_-8%,var(--section-glow-brand),transparent_34%),var(--background)]">
      <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <AppSectionTitle hostname={hostname} section="storage maintenance" />
        </div>
      </header>

      <div className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain p-3 sm:p-4">
        <div className="mx-auto grid w-full max-w-5xl gap-4 pb-4">
          <section
            aria-labelledby="storage-maintenance-title"
            className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"
          >
            <div className="flex items-start gap-3">
              <div className="grid size-9 shrink-0 place-items-center rounded-lg border border-primary/20 bg-primary/10 text-primary-text">
                <Database className="size-4" />
              </div>
              <div className="min-w-0">
                <h1 id="storage-maintenance-title" className="text-base font-semibold">
                  Storage maintenance
                </h1>
                <p className="mt-1 max-w-2xl text-[11px] leading-relaxed text-muted-foreground">
                  Remove historical runtime data deliberately. Active procedure executions remain
                  available and are never deleted by this operation.
                </p>
              </div>
            </div>
            <Button asChild variant="outline" className="min-h-11 w-full shrink-0 sm:w-auto">
              <Link
                to="/settings/$section"
                params={{ section: 'storage' }}
                aria-label="Back to Storage settings"
              >
                <ArrowLeft className="size-3.5" aria-hidden="true" />
                Back to Storage
              </Link>
            </Button>
          </section>

          {statsQuery.isLoading && <StorageMaintenanceLoading />}

          {statsQuery.isError && stats == null && (
            <StorageMaintenanceError
              message={
                statsQuery.error instanceof Error
                  ? statsQuery.error.message
                  : 'Storage statistics are unavailable.'
              }
              onRetry={() => void statsQuery.refetch()}
            />
          )}

          {stats != null && (
            <>
              <section className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(18rem,0.7fr)]">
                <Card>
                  <CardHeader>
                    <CardTitle>Cleanup scope</CardTitle>
                    <CardDescription>
                      Choose one history resource or clear every eligible resource atomically.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-3">
                    <div className="grid gap-1.5">
                      <label
                        htmlFor="storage-resource"
                        className="text-[11px] text-muted-foreground"
                      >
                        Resource
                      </label>
                      <Select value={selectedResource} onValueChange={setSelectedResource}>
                        <SelectTrigger
                          id="storage-resource"
                          aria-label="Storage resource"
                          className="min-h-11 w-full bg-surface-overlay"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={allResources}>All eligible history</SelectItem>
                          {stats.resources.map((resource) => (
                            <SelectItem key={resource.resource} value={resource.resource}>
                              {resource.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="grid grid-cols-2 gap-2">
                      <ImpactMetric
                        label="Will remove"
                        value={selectedImpact.flushableRows}
                        tone="destructive"
                      />
                      <ImpactMetric
                        label="Will preserve"
                        value={selectedImpact.protectedRows}
                        tone="protected"
                      />
                    </div>
                    <Button
                      variant="destructive"
                      className="min-h-11 w-full"
                      disabled={selectedImpact.flushableRows === 0 || flushingResource !== ''}
                      onClick={() => setConfirmResource(selectedResource)}
                    >
                      <Trash2 className="size-3.5" />
                      {flushingResource === selectedResource
                        ? 'Clearing eligible data...'
                        : 'Clear eligible data'}
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>SQLite footprint</CardTitle>
                    <CardDescription>
                      File sizes reflect the current database, WAL, and shared-memory files.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-2">
                    <p className="font-mono text-xl font-semibold text-foreground">
                      {formatBytes(stats.totalBytes)}
                    </p>
                    <div className="grid gap-1 text-[11px] text-secondary-foreground">
                      <span>database · {formatBytes(stats.databaseBytes)}</span>
                      <span>wal · {formatBytes(stats.walBytes)}</span>
                      <span>shm · {formatBytes(stats.shmBytes)}</span>
                    </div>
                    <Button
                      variant="outline"
                      className="mt-auto min-h-11 w-full"
                      disabled={statsQuery.isFetching || flushingResource !== ''}
                      onClick={() => void statsQuery.refetch()}
                    >
                      <RefreshCw
                        className={`size-3.5 ${statsQuery.isFetching ? 'motion-safe:animate-spin' : ''}`}
                      />
                      Refresh statistics
                    </Button>
                  </CardContent>
                </Card>
              </section>

              {flushError !== '' && (
                <div
                  role="alert"
                  className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-[11px] text-destructive-foreground"
                >
                  {flushError}
                </div>
              )}
              {notice !== '' && (
                <div
                  role="status"
                  className="rounded-lg border border-ok/35 bg-ok/10 p-3 text-[11px] text-ok-foreground"
                >
                  {notice}
                </div>
              )}

              {totalFlushable === 0 && (
                <EmptyState variant="inline" className="p-4">
                  <ShieldCheck className="mx-auto mb-2 size-5 text-ok" />
                  <p className="font-medium text-foreground">
                    No historical rows are eligible for removal.
                  </p>
                  <p className="mt-1 text-[11px]">
                    Protected active jobs remain visible until they reach a terminal state.
                  </p>
                </EmptyState>
              )}

              <section aria-label="Storage resources" className="grid gap-3 md:grid-cols-2">
                {stats.resources.map((resource) => (
                  <StorageResourceCard key={resource.resource} resource={resource} />
                ))}
              </section>
            </>
          )}
        </div>
      </div>

      <AlertDialog
        open={confirmResource != null}
        onOpenChange={(open) => {
          if (!open) setConfirmResource(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear eligible historical data?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes {formatRows(confirmImpact.flushableRows)} eligible row
              {confirmImpact.flushableRows === 1 ? '' : 's'} from{' '}
              {confirmResource === allResources ? 'all resources' : confirmImpact.label}.{' '}
              {formatRows(confirmImpact.protectedRows)} active row
              {confirmImpact.protectedRows === 1 ? '' : 's'} will be preserved. This cannot be
              undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="min-h-11">Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              className="min-h-11"
              onClick={() => {
                if (confirmResource != null) {
                  void executeFlush(confirmResource)
                }
                setConfirmResource(null)
              }}
            >
              Clear eligible rows
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </main>
  )
}

export function StorageMaintenanceLoading() {
  return (
    <div aria-label="Loading storage maintenance" className="grid gap-3 md:grid-cols-2">
      <div className="h-64 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-surface-raised" />
      <div className="h-64 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-surface-raised" />
    </div>
  )
}

export function StorageMaintenanceError({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <EmptyState variant="inline" className="grid gap-3 p-5">
      <div>
        <p className="font-medium text-destructive-foreground">Storage data is unavailable</p>
        <p className="mt-1 text-[11px]">{message}</p>
      </div>
      <Button variant="outline" className="mx-auto min-h-11" onClick={onRetry}>
        Try again
      </Button>
    </EmptyState>
  )
}

function StorageResourceCard({ resource }: { resource: StorageResourceStat }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {resource.label}
          {resource.protectedRows > 0 && (
            <Badge variant="outline" className="border-ok/30 text-ok-foreground">
              <ShieldCheck className="size-3" />
              active rows protected
            </Badge>
          )}
        </CardTitle>
        <CardDescription>{resource.resource}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <div className="grid grid-cols-3 gap-2">
          <ResourceMetric label="Total" value={resource.totalRows} />
          <ResourceMetric label="Eligible" value={resource.flushableRows} />
          <ResourceMetric label="Protected" value={resource.protectedRows} />
        </div>
        <div className="flex items-center justify-between gap-2 border-t border-border-subtle pt-3 text-[11px] text-muted-foreground">
          <span>Approximate payload</span>
          <span className="font-mono text-secondary-foreground">
            {formatBytes(resource.approxBytes)}
          </span>
        </div>
        {resource.resource === 'ops-jobs' && (
          <p className="rounded-md border border-ok/25 bg-ok/8 p-2 text-[10px] leading-relaxed text-ok-foreground">
            Queued, running, and approval-waiting jobs stay intact. Only succeeded and failed jobs
            are eligible.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function ResourceMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="min-w-0 rounded-md border border-border-subtle bg-surface-overlay p-2">
      <p className="truncate text-[10px] text-muted-foreground">{label}</p>
      <p className="mt-1 font-mono text-sm font-semibold text-foreground">{formatRows(value)}</p>
    </div>
  )
}

function ImpactMetric({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone: 'destructive' | 'protected'
}) {
  return (
    <div
      className={
        tone === 'destructive'
          ? 'rounded-md border border-destructive/30 bg-destructive/8 p-2'
          : 'rounded-md border border-ok/25 bg-ok/8 p-2'
      }
    >
      <p className="text-[10px] text-muted-foreground">{label}</p>
      <p className="mt-1 font-mono text-sm font-semibold text-foreground">
        {formatRows(value)} rows
      </p>
    </div>
  )
}

function storageImpact(resources: Array<StorageResourceStat>, resource: string) {
  if (resource === allResources) {
    return {
      label: 'all resources',
      flushableRows: sumRows(resources, 'flushableRows'),
      protectedRows: sumRows(resources, 'protectedRows'),
    }
  }
  const selected = resources.find((item) => item.resource === resource)
  return {
    label: selected?.label ?? resource,
    flushableRows: selected?.flushableRows ?? 0,
    protectedRows: selected?.protectedRows ?? 0,
  }
}

function sumRows(resources: Array<StorageResourceStat>, field: 'flushableRows' | 'protectedRows') {
  return resources.reduce((total, resource) => total + resource[field], 0)
}

export function formatRows(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0'
  return new Intl.NumberFormat('en-US').format(Math.trunc(value))
}
