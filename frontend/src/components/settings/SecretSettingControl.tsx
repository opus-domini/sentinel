import type { SensitiveSetting } from '@/api/settings'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { EnvironmentOwnership, ValidationError } from './SettingsField'

export type SecretIntent = 'keep' | 'replace' | 'clear'

type SecretSettingControlProps = {
  id: string
  label: string
  setting: SensitiveSetting
  intent: SecretIntent
  value: string
  error?: string
  placeholder: string
  onIntentChange: (intent: SecretIntent) => void
  onValueChange: (value: string) => void
}

export default function SecretSettingControl({
  id,
  label,
  setting,
  intent,
  value,
  error = '',
  placeholder,
  onIntentChange,
  onValueChange,
}: SecretSettingControlProps) {
  const inputID = `${id}-replacement`
  const errorID = `${id}-error`
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span
          className={cn(
            'rounded-full border px-2 py-1 text-[10px] font-medium',
            setting.configured
              ? 'border-ok/40 bg-ok/10 text-ok-foreground'
              : 'border-border-subtle bg-surface-overlay text-muted-foreground',
          )}
        >
          {setting.configured ? 'Configured' : 'Not configured'}
        </span>
        <div
          role="group"
          aria-label={`${label} action`}
          className="grid w-full grid-cols-3 rounded-md border border-border-subtle bg-surface-overlay p-0.5 sm:w-auto"
        >
          {(['keep', 'replace', 'clear'] as const).map((action) => (
            <Button
              key={action}
              type="button"
              size="sm"
              variant="ghost"
              aria-pressed={intent === action}
              disabled={
                !setting.editable ||
                (action === 'clear' && !setting.configured && intent !== 'clear')
              }
              className={cn(
                'min-h-11 px-3 text-[10px] capitalize',
                intent === action && 'bg-primary/15 text-primary-text hover:bg-primary/20',
              )}
              onClick={() => onIntentChange(action)}
            >
              {action === 'keep' ? 'Keep' : action === 'replace' ? 'Replace' : 'Clear'}
            </Button>
          ))}
        </div>
      </div>

      {intent === 'replace' && (
        <div className="grid gap-1.5">
          <label htmlFor={inputID} className="text-[10px] font-medium text-secondary-foreground">
            New {label.toLowerCase()}
          </label>
          <Input
            id={inputID}
            name={inputID}
            type="password"
            autoComplete="new-password"
            autoCapitalize="none"
            spellCheck={false}
            value={value}
            placeholder={placeholder}
            aria-invalid={error !== '' ? true : undefined}
            aria-describedby={error !== '' ? errorID : undefined}
            disabled={!setting.editable}
            className="min-h-11 bg-surface-overlay font-mono sm:max-w-xl"
            onChange={(event) => onValueChange(event.target.value)}
          />
        </div>
      )}

      {intent === 'clear' && (
        <p className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-[10px] leading-relaxed text-warning-foreground">
          The saved value will be removed. The running process keeps its startup value until
          Sentinel restarts.
        </p>
      )}

      <ValidationError id={errorID} message={error} />
      <EnvironmentOwnership setting={setting} />
      <p className="text-[10px] leading-relaxed text-muted-foreground">
        Existing values are never returned to this browser. Replace accepts a new value once; Keep
        and Clear send no secret.
      </p>
    </div>
  )
}
