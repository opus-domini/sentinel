import { cn } from '@/lib/utils'

export function MetricCard({
  label,
  value,
  sub,
  alert,
  onClick,
  selected,
}: {
  label: string
  value: string
  sub?: string
  alert?: boolean
  onClick?: () => void
  selected?: boolean
}) {
  const className = cn(
    'rounded-lg border p-2.5 text-left',
    alert
      ? 'border-destructive/40 bg-destructive/10'
      : 'border-border-subtle bg-surface-elevated',
    selected && 'ring-1 ring-primary/50 border-primary/40',
    onClick && 'cursor-pointer hover:bg-accent/40 transition-colors',
  )

  const content = (
    <>
      <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-[12px] font-semibold">{value}</p>
      {sub && <p className="text-[10px] text-muted-foreground">{sub}</p>}
    </>
  )

  if (onClick) {
    return (
      <button type="button" className={className} onClick={onClick}>
        {content}
      </button>
    )
  }

  return <div className={className}>{content}</div>
}
