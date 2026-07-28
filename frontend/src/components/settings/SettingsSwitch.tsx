import { cn } from '@/lib/utils'

type SettingsSwitchProps = {
  id?: string
  label: string
  checked: boolean
  disabled?: boolean
  tone?: 'primary' | 'warning'
  onCheckedChange: (checked: boolean) => void
}

export default function SettingsSwitch({
  id,
  label,
  checked,
  disabled = false,
  tone = 'primary',
  onCheckedChange,
}: SettingsSwitchProps) {
  return (
    <button
      id={id}
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className="inline-flex size-11 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-surface-overlay focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
    >
      <span
        aria-hidden="true"
        className={cn(
          'relative h-5 w-9 rounded-full border transition-colors',
          checked
            ? tone === 'warning'
              ? 'border-warning/60 bg-warning/25'
              : 'border-primary/60 bg-primary/30'
            : 'border-border bg-background',
        )}
      >
        <span
          className={cn(
            'absolute top-0.5 left-0.5 size-4 rounded-full transition-transform',
            checked ? 'translate-x-4 bg-foreground' : 'bg-muted-foreground',
          )}
        />
      </span>
    </button>
  )
}
