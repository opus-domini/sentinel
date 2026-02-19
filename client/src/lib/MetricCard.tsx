import { cn } from '@/lib/utils'

export function MetricCard({
  label,
  value,
  sub,
  alert,
}: {
  label: string
  value: string
  sub?: string
  alert?: boolean
}) {
  return (
    <div
      className={cn(
        'rounded-lg border p-2.5',
        alert
          ? 'border-red-500/40 bg-red-500/10'
          : 'border-border-subtle bg-surface-elevated',
      )}
    >
      <p className="text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-[12px] font-semibold">{value}</p>
      {sub && <p className="text-[10px] text-muted-foreground">{sub}</p>}
    </div>
  )
}
