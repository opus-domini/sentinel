import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'

type SettingsSectionHeaderProps = {
  title: string
  description: string
  icon: ReactNode
}

export default function SettingsSectionHeader({
  title,
  description,
  icon,
}: SettingsSectionHeaderProps) {
  const headingRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    headingRef.current?.focus({ preventScroll: true })
  }, [])

  return (
    <header className="flex items-start gap-3">
      <span className="grid size-9 shrink-0 place-items-center rounded-lg border border-primary/20 bg-primary/10 text-primary-text">
        {icon}
      </span>
      <div className="min-w-0">
        <h1 ref={headingRef} tabIndex={-1} className="text-base font-semibold outline-none">
          {title}
        </h1>
        <p className="mt-1 max-w-2xl text-[11px] leading-relaxed text-muted-foreground">
          {description}
        </p>
      </div>
    </header>
  )
}
