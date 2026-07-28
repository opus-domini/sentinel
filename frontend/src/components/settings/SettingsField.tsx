import type { ReactNode } from 'react'

import type { BooleanSetting, SettingApplyMode, SettingSource, StringSetting } from '@/api/settings'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

type SettingMetadata = Pick<
  StringSetting | BooleanSetting,
  'source' | 'applyMode' | 'restartPending'
>

type SettingsFieldProps = {
  label: string
  description: string
  setting?: SettingMetadata
  ownership?: 'browser' | 'device'
  htmlFor?: string
  children: ReactNode
  className?: string
}

export function SettingsField({
  label,
  description,
  setting,
  ownership,
  htmlFor,
  children,
  className,
}: SettingsFieldProps) {
  const Label = htmlFor == null ? 'h2' : 'label'
  return (
    <section
      className={cn(
        'grid gap-3 rounded-lg border border-border-subtle bg-card p-3 sm:p-4',
        className,
      )}
    >
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <Label htmlFor={htmlFor} className="text-[12px] font-medium text-foreground">
            {label}
          </Label>
          <p className="mt-1 max-w-2xl text-[11px] leading-relaxed text-muted-foreground">
            {description}
          </p>
        </div>
        <FieldMetadata setting={setting} ownership={ownership} />
      </div>
      {children}
    </section>
  )
}

export function FieldMetadata({
  setting,
  ownership,
}: {
  setting?: SettingMetadata
  ownership?: 'browser' | 'device'
}) {
  if (setting == null && ownership == null) return null
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
      {ownership && (
        <Badge variant="outline" className="border-border-subtle text-[9px] text-muted-foreground">
          {ownership === 'browser' ? 'This browser' : 'This device'}
        </Badge>
      )}
      {setting && (
        <>
          <SourceBadge source={setting.source} />
          <ApplyBadge mode={setting.applyMode} restartPending={setting.restartPending} />
        </>
      )}
    </div>
  )
}

export function SourceBadge({ source }: { source: SettingSource }) {
  const labels: Record<SettingSource, string> = {
    default: 'Default',
    file: 'Config file',
    environment: 'Environment',
  }
  return (
    <Badge variant="outline" className="border-border-subtle text-[9px] text-muted-foreground">
      {labels[source]}
    </Badge>
  )
}

export function ApplyBadge({
  mode,
  restartPending,
}: {
  mode: SettingApplyMode
  restartPending: boolean
}) {
  const labels: Record<SettingApplyMode, string> = {
    live: 'Live',
    partial: 'Partially live',
    restart: 'After restart',
  }
  return (
    <Badge
      variant="outline"
      className={cn(
        'text-[9px]',
        restartPending
          ? 'border-warning/45 bg-warning/10 text-warning-foreground'
          : mode === 'live'
            ? 'border-ok/35 bg-ok/10 text-ok-foreground'
            : 'border-primary/30 bg-primary/10 text-primary-text',
      )}
    >
      {restartPending ? 'Restart pending' : labels[mode]}
    </Badge>
  )
}

export function ValidationError({ message, id }: { message: string; id?: string }) {
  if (message === '') return null
  return (
    <p
      id={id}
      role="alert"
      className="rounded-md border border-destructive/45 bg-destructive/10 px-3 py-2 text-[11px] leading-relaxed text-destructive-foreground"
    >
      {message}
    </p>
  )
}

export function SaveFeedback({
  status,
  message,
}: {
  status: 'idle' | 'saving' | 'success' | 'error'
  message: string
}) {
  return (
    <div aria-live="polite" aria-atomic="true" className="min-h-4 text-[10px]">
      {status !== 'idle' && (
        <span
          className={cn(
            status === 'error'
              ? 'text-destructive-foreground'
              : status === 'success'
                ? 'text-ok-foreground'
                : 'text-muted-foreground',
          )}
        >
          {message}
        </span>
      )}
    </div>
  )
}
