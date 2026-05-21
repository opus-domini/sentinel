import { cn } from '@/lib/utils'

export function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(percent, 100)
  const color =
    percent > 90 ? 'bg-destructive' : percent > 80 ? 'bg-warning' : 'bg-ok'

  return (
    <div
      className="mt-1.5 h-1.5 w-full rounded-full bg-surface-overlay"
      role="progressbar"
      aria-valuenow={clamped}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div
        className={cn('h-1.5 rounded-full', color)}
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}
