import { cn } from '@/lib/utils'

export function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(percent, 100)
  const color =
    percent > 90
      ? 'bg-red-500'
      : percent > 80
        ? 'bg-amber-500'
        : 'bg-emerald-500'

  return (
    <div className="mt-1.5 h-1.5 w-full rounded-full bg-surface-overlay">
      <div
        className={cn('h-1.5 rounded-full', color)}
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}
