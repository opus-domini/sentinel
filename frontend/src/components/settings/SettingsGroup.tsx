import type { ReactNode } from 'react'

type SettingsGroupProps = {
  title: string
  description: string
  icon: ReactNode
  children: ReactNode
}

export default function SettingsGroup({ title, description, icon, children }: SettingsGroupProps) {
  return (
    <section className="grid gap-3">
      <div className="flex items-start gap-2 px-1">
        <span className="mt-0.5 text-primary">{icon}</span>
        <div>
          <h2 className="text-[12px] font-medium">{title}</h2>
          <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="grid gap-3">{children}</div>
    </section>
  )
}
