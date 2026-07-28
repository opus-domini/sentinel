import { useState } from 'react'
import { Check, Copy, RotateCcw } from 'lucide-react'

import type { SettingsResponse } from '@/api/settings'
import { Button } from '@/components/ui/button'
import { writeClipboardText } from '@/lib/clipboardProvider'

type RestartPendingNoticeProps = {
  restart: SettingsResponse['restart']
  deployment?: SettingsResponse['deployment']
}

export default function RestartPendingNotice({ restart, deployment }: RestartPendingNoticeProps) {
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'error'>('idle')
  if (!restart.required) return null
  return (
    <aside
      aria-label="Restart required"
      className="rounded-lg border border-warning/45 bg-warning/10 p-3 text-warning-foreground"
    >
      <div className="flex items-start gap-2">
        <RotateCcw className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div className="min-w-0">
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
                {copyState === 'copied' ? 'Copied' : 'Copy'}
              </Button>
            </div>
          )}
          <div aria-live="polite" className="min-h-4 text-[10px]">
            {copyState === 'error' && 'Clipboard access failed. Select the command manually.'}
          </div>
        </div>
      </div>
    </aside>
  )
}
