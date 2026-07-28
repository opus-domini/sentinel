import { Link } from '@tanstack/react-router'

import type { SettingsSection } from './settingsSections'
import { SETTINGS_SECTIONS } from './settingsSections'
import { cn } from '@/lib/utils'

type SettingsNavigationProps = {
  activeSection: SettingsSection
}

export default function SettingsNavigation({ activeSection }: SettingsNavigationProps) {
  return (
    <nav aria-label="Settings sections" className="min-w-0">
      <ul className="grid grid-cols-2 gap-1.5 md:grid-cols-1 md:gap-1">
        {SETTINGS_SECTIONS.map(({ id, label, description, Icon }) => {
          const active = id === activeSection
          return (
            <li key={id}>
              <Link
                to="/settings/$section"
                params={{ section: id }}
                aria-current={active ? 'page' : undefined}
                className={cn(
                  'group relative flex min-h-11 items-start gap-2 rounded-md border px-2.5 py-2.5 no-underline transition-colors md:gap-3 md:px-3',
                  active
                    ? 'border-primary/25 bg-primary/10 text-primary-text-bright before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-primary'
                    : 'border-transparent text-secondary-foreground hover:border-border-subtle hover:bg-surface-hover hover:text-foreground',
                )}
              >
                <Icon
                  className={cn(
                    'mt-0.5 size-4 shrink-0',
                    active ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground',
                  )}
                  aria-hidden="true"
                />
                <span className="min-w-0">
                  <span className="block text-[12px] font-medium">{label}</span>
                  <span className="mt-0.5 block text-[10px] leading-relaxed text-muted-foreground">
                    {description}
                  </span>
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
