import { useId, useState } from 'react'

import { cn } from '@/lib/utils'

type SparklineProps = {
  data: Array<number>
  timestamps?: Array<number>
  color: string
  fill?: boolean
  className?: string
  formatValue?: (v: number) => string
  domain?: [number, number]
}

function formatRelativeTime(ts: number): string {
  const diff = Math.round((Date.now() - ts) / 1000)
  if (diff < 10) return 'just now'
  if (diff < 60) return `${diff}s ago`
  const m = Math.floor(diff / 60)
  const s = diff % 60
  if (m < 60) return s > 0 ? `${m}m ${s}s ago` : `${m}m ago`
  return `${Math.floor(m / 60)}h ${m % 60}m ago`
}

export function Sparkline({
  data,
  timestamps,
  color,
  fill = true,
  className,
  formatValue,
  domain,
}: SparklineProps) {
  const id = useId()
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)

  if (data.length < 2) return null

  const min = domain ? domain[0] : Math.min(...data)
  const max = domain ? domain[1] : Math.max(...data)
  const range = max - min || 1

  const pad = 2
  const h = 40 - pad * 2
  const w = 200
  const step = w / (data.length - 1)

  const coords = data.map((v, i) => ({
    x: i * step,
    y: pad + h - ((v - min) / range) * h,
  }))

  const polylinePoints = coords.map((p) => `${p.x},${p.y}`).join(' ')
  const gradientId = `sparkline-fill-${id}`

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - rect.left
    const fraction = x / rect.width
    const index = Math.round(fraction * (data.length - 1))
    setHoverIndex(Math.max(0, Math.min(data.length - 1, index)))
  }

  const handleMouseLeave = () => setHoverIndex(null)

  const hoverXPct =
    hoverIndex !== null ? (hoverIndex / (data.length - 1)) * 100 : 0
  const hoverYPct = hoverIndex !== null ? (coords[hoverIndex].y / 40) * 100 : 0

  return (
    <div
      className={cn('relative', className)}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
    >
      <svg
        viewBox="0 0 200 40"
        preserveAspectRatio="none"
        className="h-full w-full"
      >
        {fill && (
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.25} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
        )}
        {fill && (
          <polygon
            points={`0,40 ${polylinePoints} ${w},40`}
            fill={`url(#${gradientId})`}
          />
        )}
        <polyline
          points={polylinePoints}
          fill="none"
          stroke={color}
          strokeWidth={1.5}
          vectorEffect="non-scaling-stroke"
        />
      </svg>

      {hoverIndex !== null && (
        <>
          <div
            className="absolute top-0 bottom-0 w-px"
            style={{ left: `${hoverXPct}%`, backgroundColor: `${color}40` }}
          />
          <div
            className="absolute h-[7px] w-[7px] rounded-full"
            style={{
              left: `${hoverXPct}%`,
              top: `${hoverYPct}%`,
              backgroundColor: color,
              transform: 'translate(-50%, -50%)',
              boxShadow: '0 0 0 1.5px var(--background)',
            }}
          />
          <div
            className="pointer-events-none absolute bottom-[calc(100%+4px)] z-10 flex items-center gap-1 whitespace-nowrap rounded bg-popover px-1.5 py-0.5 text-[10px] text-popover-foreground shadow-md"
            style={{
              left: `clamp(0px, calc(${hoverXPct}% - 40px), calc(100% - 80px))`,
            }}
          >
            <span style={{ color }}>
              {formatValue ? formatValue(data[hoverIndex]) : data[hoverIndex]}
            </span>
            {timestamps?.[hoverIndex] != null && (
              <span className="text-muted-foreground">
                · {formatRelativeTime(timestamps[hoverIndex])}
              </span>
            )}
          </div>
        </>
      )}
    </div>
  )
}
