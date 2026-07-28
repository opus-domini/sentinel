import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Check, Copy, LoaderCircle, RotateCcw } from 'lucide-react'

import { restartSettings } from '@/api/settings'
import type { SettingsResponse } from '@/api/settings'
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
import { Button } from '@/components/ui/button'
import { useToastContext } from '@/contexts/ToastContext'
import { SETTINGS_QUERY_KEY } from '@/hooks/useSettings'
import { ApiError } from '@/hooks/useTmuxApi'
import { writeClipboardText } from '@/lib/clipboardProvider'
import {
  navigateToRestartTarget,
  reloadForAuthentication,
  settingsReconnectTarget,
  waitForRestartTarget,
  waitForSettingsRestart,
} from '@/lib/settingsRestart'

type RestartPendingNoticeProps = {
  revision: string
  restart: SettingsResponse['restart']
  deployment?: SettingsResponse['deployment']
  reconnectOrigin?: string
  onRestartComplete?: () => void
}

type RestartPhase = 'idle' | 'requesting' | 'waiting' | 'relocating' | 'error'

export default function RestartPendingNotice(props: RestartPendingNoticeProps) {
  if (!props.restart.required) return null
  return <RestartPendingContent {...props} />
}

function RestartPendingContent({
  revision,
  restart,
  deployment,
  reconnectOrigin,
  onRestartComplete,
}: RestartPendingNoticeProps) {
  const queryClient = useQueryClient()
  const { pushToast } = useToastContext()
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'error'>('idle')
  const [confirmationOpen, setConfirmationOpen] = useState(false)
  const [phase, setPhase] = useState<RestartPhase>('idle')
  const [restartError, setRestartError] = useState('')

  const busy = phase === 'requesting' || phase === 'waiting' || phase === 'relocating'

  const monitorRestart = async () => {
    setPhase('waiting')
    setRestartError('')
    try {
      const result = await waitForSettingsRestart()
      if (result.status === 'authentication-required') {
        reloadForAuthentication()
        return
      }
      onRestartComplete?.()
      queryClient.setQueryData(SETTINGS_QUERY_KEY, result.snapshot)
      void queryClient.invalidateQueries({ queryKey: ['meta'], exact: true })
      setPhase('idle')
      pushToast({
        level: 'success',
        title: 'Sentinel restarted',
        message: 'Saved settings are active and the service is responding again.',
      })
    } catch (error) {
      setPhase('error')
      setRestartError(
        error instanceof Error
          ? `${error.message} Check the service or use the command below.`
          : 'Sentinel did not return. Check the service or use the command below.',
      )
    }
  }

  const confirmRestart = async () => {
    setConfirmationOpen(false)
    setPhase('requesting')
    setRestartError('')
    try {
      await restartSettings(revision)
    } catch (error) {
      if (error instanceof ApiError && error.code === 'RESTART_ALREADY_SCHEDULED') {
        await monitorRestart()
        return
      }
      if (error instanceof ApiError && error.code === 'CONFIG_CONFLICT') {
        void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY, exact: true })
      }
      setPhase('error')
      setRestartError(
        error instanceof Error ? error.message : 'Sentinel could not schedule the service restart.',
      )
      return
    }

    const target = settingsReconnectTarget(reconnectOrigin)
    if (target !== '') {
      setPhase('relocating')
      try {
        await waitForRestartTarget(target)
        navigateToRestartTarget(target)
      } catch (error) {
        setPhase('error')
        setRestartError(
          error instanceof Error
            ? `${error.message} Open ${target} manually or use the recovery command below.`
            : `The saved endpoint did not return. Open ${target} manually or use the recovery command below.`,
        )
      }
      return
    }
    await monitorRestart()
  }

  return (
    <>
      <aside
        aria-label="Restart required"
        className="rounded-lg border border-warning/45 bg-warning/10 p-3 text-warning-foreground"
      >
        <div className="flex items-start gap-2">
          <RotateCcw className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h2 className="text-[12px] font-medium">Restart required</h2>
            <p className="mt-1 text-[11px] leading-relaxed">{restart.instruction}</p>
            {deployment && (
              <p className="mt-2 text-[10px] text-warning-foreground/80">
                Deployment scope · <span className="font-mono">{deployment.scope}</span>
              </p>
            )}
            {restart.changedKeys.length > 0 && (
              <p className="mt-2 break-words font-mono text-[10px]">
                {restart.changedKeys.join(' · ')}
              </p>
            )}

            {busy && (
              <div
                role="status"
                className="mt-3 flex items-start gap-2 rounded-md border border-warning/30 bg-background/35 p-2.5 text-[10px] leading-relaxed"
              >
                <LoaderCircle
                  className="mt-0.5 size-3.5 shrink-0 motion-safe:animate-spin"
                  aria-hidden="true"
                />
                <span>
                  {phase === 'requesting' && 'Scheduling the managed service restart…'}
                  {phase === 'waiting' &&
                    'Sentinel is restarting. This page will reconnect automatically.'}
                  {phase === 'relocating' &&
                    'Sentinel is restarting. Moving this browser to the saved endpoint…'}
                </span>
              </div>
            )}
            {restartError !== '' && (
              <p
                role="alert"
                className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 p-2.5 text-[10px] leading-relaxed text-destructive-foreground"
              >
                {restartError}
              </p>
            )}

            <div className="mt-3 grid gap-2 sm:flex sm:flex-wrap">
              {restart.available && (
                <Button
                  type="button"
                  className="min-h-11 w-full sm:w-auto"
                  disabled={busy}
                  onClick={() => setConfirmationOpen(true)}
                >
                  {busy ? (
                    <LoaderCircle
                      className="size-3.5 motion-safe:animate-spin"
                      aria-hidden="true"
                    />
                  ) : (
                    <RotateCcw className="size-3.5" aria-hidden="true" />
                  )}
                  {busy ? 'Restarting Sentinel…' : 'Restart Sentinel'}
                </Button>
              )}
              {phase === 'error' && restart.available && (
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 w-full border-warning/30 bg-background/35 sm:w-auto"
                  onClick={() => void monitorRestart()}
                >
                  Check again
                </Button>
              )}
            </div>

            {restart.command && (
              <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
                <code className="block overflow-x-auto rounded border border-warning/25 bg-background/40 p-2 text-[10px] whitespace-nowrap">
                  {restart.command}
                </code>
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 w-full border-warning/30 bg-background/35 sm:w-auto"
                  onClick={() => {
                    setCopyState('idle')
                    void writeClipboardText(restart.command ?? '').then((copied) => {
                      setCopyState(copied ? 'copied' : 'error')
                    })
                  }}
                >
                  {copyState === 'copied' ? (
                    <Check className="size-3.5" aria-hidden="true" />
                  ) : (
                    <Copy className="size-3.5" aria-hidden="true" />
                  )}
                  {copyState === 'copied' ? 'Copied' : 'Copy command'}
                </Button>
              </div>
            )}
            <div aria-live="polite" className="min-h-4 text-[10px]">
              {copyState === 'error' && 'Clipboard access failed. Select the command manually.'}
            </div>
          </div>
        </div>
      </aside>

      <AlertDialog open={confirmationOpen} onOpenChange={setConfirmationOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Restart Sentinel now?</AlertDialogTitle>
            <AlertDialogDescription>
              The service will be briefly unavailable while the saved configuration becomes active.
              This page will reconnect automatically.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid max-h-[45vh] gap-3 overflow-y-auto text-[10px]">
            <div className="rounded-md border border-border-subtle bg-surface-overlay p-3">
              <p className="uppercase tracking-[0.08em] text-muted-foreground">
                Pending configuration
              </p>
              <ul className="mt-2 grid gap-1 font-mono text-secondary-foreground">
                {restart.changedKeys.map((key) => (
                  <li key={key} className="break-words">
                    {key}
                  </li>
                ))}
              </ul>
            </div>
            <p className="rounded-md border border-ok/30 bg-ok/10 p-3 leading-relaxed text-ok-foreground">
              Existing tmux sessions are not targeted by this service restart.
            </p>
            {reconnectOrigin && (
              <div className="rounded-md border border-primary/25 bg-primary/10 p-3">
                <p className="uppercase tracking-[0.08em] text-primary-text">Reconnect endpoint</p>
                <p className="mt-1 break-all font-mono text-secondary-foreground">
                  {reconnectOrigin}
                </p>
              </div>
            )}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel className="min-h-11">Keep running</AlertDialogCancel>
            <AlertDialogAction className="min-h-11" onClick={() => void confirmRestart()}>
              <RotateCcw className="size-3.5" aria-hidden="true" />
              Restart Sentinel
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
