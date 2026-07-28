import { RotateCcw } from 'lucide-react'

import type { SettingsResponse } from '@/api/settings'

type RestartPendingNoticeProps = {
  restart: SettingsResponse['restart']
}

export default function RestartPendingNotice({ restart }: RestartPendingNoticeProps) {
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
          {restart.changedKeys.length > 0 && (
            <p className="mt-2 break-words font-mono text-[10px]">
              {restart.changedKeys.join(' · ')}
            </p>
          )}
          {restart.command && (
            <code className="mt-2 block overflow-x-auto rounded border border-warning/25 bg-background/40 p-2 text-[10px] whitespace-nowrap">
              {restart.command}
            </code>
          )}
        </div>
      </div>
    </aside>
  )
}
