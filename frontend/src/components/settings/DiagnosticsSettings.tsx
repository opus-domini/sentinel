import { useCallback } from 'react'
import { CircleCheck, Download, RefreshCw, Wrench } from 'lucide-react'

import RestartPendingNotice from './RestartPendingNotice'
import SettingsSectionHeader from './SettingsSectionHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useMetaContext } from '@/contexts/MetaContext'
import { useToastContext } from '@/contexts/ToastContext'
import { usePwaInstall } from '@/hooks/usePwaInstall'
import { useSettings } from '@/hooks/useSettings'
import { cn } from '@/lib/utils'

export default function DiagnosticsSettings() {
  const meta = useMetaContext()
  const { pushToast } = useToastContext()
  const settingsQuery = useSettings()
  const settings = settingsQuery.settings
  const {
    supportsPwa,
    installed,
    installAvailable,
    installApp,
    updateAvailable,
    checkForUpdate,
    updating,
  } = usePwaInstall()

  const handleUpdateApp = useCallback(async () => {
    const result = await checkForUpdate()
    if (result === 'applied') return
    if (result === 'no-update') {
      window.location.reload()
      return
    }
    pushToast({
      level: 'error',
      title: 'Update unavailable',
      message:
        result === 'unsupported'
          ? 'Service workers require HTTPS or localhost.'
          : 'Failed to reach the update channel. Check connectivity.',
    })
  }, [checkForUpdate, pushToast])

  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Diagnostics"
        description="Inspect this app, its runtime ownership, and the configuration source used by the running deployment."
        icon={<Wrench className="size-4" aria-hidden="true" />}
      />

      {settingsQuery.isLoading && settings == null && (
        <div
          aria-label="Loading diagnostics"
          className="h-40 motion-safe:animate-pulse rounded-lg border border-border-subtle bg-card"
        />
      )}

      {settingsQuery.isError && settings == null && (
        <div
          role="alert"
          className="grid gap-3 rounded-lg border border-destructive/45 bg-card p-3"
        >
          <p className="text-[11px] text-destructive-foreground">
            {settingsQuery.error instanceof Error
              ? settingsQuery.error.message
              : 'Diagnostics could not be loaded.'}
          </p>
          <Button
            variant="outline"
            className="min-h-11 w-full sm:w-fit"
            onClick={() => void settingsQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      )}

      {settings && (
        <>
          <RestartPendingNotice
            revision={settings.revision}
            restart={settings.restart}
            deployment={settings.deployment}
          />

          <section className="grid gap-3 rounded-lg border border-border-subtle bg-card p-3 sm:p-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <h2 className="text-[12px] font-medium">Runtime</h2>
                <p className="mt-1 text-[11px] text-muted-foreground">
                  Version and deployment detected by the running process.
                </p>
              </div>
              <Badge
                variant="outline"
                className={cn(
                  'text-[9px]',
                  settings.diagnostics.deploymentDetection === 'matched'
                    ? 'border-ok/35 bg-ok/10 text-ok-foreground'
                    : 'border-border-subtle text-muted-foreground',
                )}
              >
                {settings.diagnostics.deploymentDetection}
              </Badge>
            </div>
            <dl className="grid gap-2 text-[11px] sm:grid-cols-2">
              <DiagnosticValue label="Version" value={settings.metadata.version || meta.version} />
              <DiagnosticValue label="Runtime mode" value={settings.deployment.runtimeMode} />
              <DiagnosticValue label="Deployment scope" value={settings.deployment.scope} />
              <DiagnosticValue
                label="Config file"
                value={settings.diagnostics.configExists ? 'Present' : 'Not created yet'}
              />
            </dl>
            <div className="min-w-0 border-t border-border-subtle pt-3">
              <p className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">
                Config path
              </p>
              <code className="mt-1 block overflow-x-auto rounded-md bg-background px-3 py-2 text-[10px] whitespace-nowrap text-secondary-foreground">
                {settings.deployment.configPath}
              </code>
            </div>
            {(settings.diagnostics.environmentOwnedKeys.length > 0 ||
              settings.diagnostics.readOnlyKeys.length > 0) && (
              <div className="grid gap-2 border-t border-border-subtle pt-3 text-[10px] text-muted-foreground">
                {settings.diagnostics.environmentOwnedKeys.length > 0 && (
                  <p>
                    Environment owned ·{' '}
                    <span className="break-words font-mono">
                      {settings.diagnostics.environmentOwnedKeys.join(', ')}
                    </span>
                  </p>
                )}
                {settings.diagnostics.readOnlyKeys.length > 0 && (
                  <p>
                    Read only ·{' '}
                    <span className="break-words font-mono">
                      {settings.diagnostics.readOnlyKeys.join(', ')}
                    </span>
                  </p>
                )}
              </div>
            )}
          </section>
        </>
      )}

      <section className="grid gap-3 rounded-lg border border-border-subtle bg-card p-3 sm:p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h2 className="text-[12px] font-medium">Progressive app</h2>
            <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
              Install this Sentinel on the device or refresh its cached application shell.
            </p>
          </div>
          <Badge
            variant="outline"
            className={cn(
              'text-[9px]',
              installed
                ? 'border-ok/35 bg-ok/10 text-ok-foreground'
                : 'border-border-subtle text-muted-foreground',
            )}
          >
            {installed ? 'Installed' : 'Browser'}
          </Badge>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            type="button"
            className="min-h-11 w-full sm:w-auto"
            onClick={() => void installApp()}
            disabled={!installAvailable}
          >
            <Download className="size-3.5" aria-hidden="true" />
            Install app
          </Button>
          <Button
            type="button"
            variant="outline"
            className="min-h-11 w-full sm:w-auto"
            onClick={() => void handleUpdateApp()}
            disabled={!supportsPwa || updating}
          >
            {updateAvailable ? (
              <CircleCheck className="size-3.5" aria-hidden="true" />
            ) : (
              <RefreshCw
                className={cn('size-3.5', updating && 'motion-safe:animate-spin')}
                aria-hidden="true"
              />
            )}
            {updating ? 'Updating…' : updateAvailable ? 'Apply update' : 'Check for update'}
          </Button>
        </div>
        {!supportsPwa && (
          <p className="text-[10px] text-warning-foreground">
            App installation and updates require HTTPS or localhost.
          </p>
        )}
      </section>
    </div>
  )
}

function DiagnosticValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border-subtle bg-surface-overlay p-3">
      <dt className="text-[9px] uppercase tracking-[0.08em] text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-mono text-[11px] text-foreground">{value || '—'}</dd>
    </div>
  )
}
