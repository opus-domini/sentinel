import { Link, useNavigate } from '@tanstack/react-router'

import type { SettingsSection } from './settingsSections'
import { isSettingsSection, SETTINGS_SECTIONS } from './settingsSections'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

type SettingsNavigationProps = {
  activeSection: SettingsSection
}

export default function SettingsNavigation({ activeSection }: SettingsNavigationProps) {
  const navigate = useNavigate()
  const active = SETTINGS_SECTIONS.find((section) => section.id === activeSection)

  return (
    <>
      <div className="min-w-0 md:hidden">
        <Select
          value={activeSection}
          onValueChange={(value) => {
            if (!isSettingsSection(value) || value === activeSection) return
            void navigate({
              to: '/settings/$section',
              params: { section: value },
            })
          }}
        >
          <SelectTrigger
            aria-label="Settings section"
            className="h-auto min-h-16 w-full bg-surface-overlay px-3 py-2 text-left whitespace-normal *:data-[slot=select-value]:min-w-0 *:data-[slot=select-value]:flex-1"
          >
            <SelectValue>{active && <SettingsSectionChoice section={active} active />}</SelectValue>
          </SelectTrigger>
          <SelectContent
            position="popper"
            align="start"
            sideOffset={4}
            className="max-w-[calc(100vw-1.5rem)] min-w-[var(--radix-select-trigger-width)] motion-reduce:animate-none"
          >
            {SETTINGS_SECTIONS.map((section) => (
              <SelectItem
                key={section.id}
                value={section.id}
                className="min-h-14 py-2 pr-9 whitespace-normal"
              >
                <SettingsSectionChoice section={section} active={section.id === activeSection} />
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <nav aria-label="Settings sections" className="hidden min-w-0 md:block">
        <ul className="grid gap-1">
          {SETTINGS_SECTIONS.map(({ id, label, description, Icon }) => {
            const sectionActive = id === activeSection
            return (
              <li key={id}>
                <Link
                  to="/settings/$section"
                  params={{ section: id }}
                  aria-current={sectionActive ? 'page' : undefined}
                  className={cn(
                    'group relative flex min-h-11 items-start gap-3 rounded-md border px-3 py-2.5 no-underline transition-colors',
                    sectionActive
                      ? 'border-primary/25 bg-primary/10 text-primary-text-bright before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-primary'
                      : 'border-transparent text-secondary-foreground hover:border-border-subtle hover:bg-surface-hover hover:text-foreground',
                  )}
                >
                  <Icon
                    className={cn(
                      'mt-0.5 size-4 shrink-0',
                      sectionActive
                        ? 'text-primary'
                        : 'text-muted-foreground group-hover:text-foreground',
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
    </>
  )
}

type SettingsSectionDefinition = (typeof SETTINGS_SECTIONS)[number]

function SettingsSectionChoice({
  section,
  active,
}: {
  section: SettingsSectionDefinition
  active: boolean
}) {
  const { label, description, Icon } = section

  return (
    <span className="flex min-w-0 flex-1 items-center gap-2.5 text-left">
      <span
        className={cn(
          'grid size-8 shrink-0 place-items-center rounded-md border',
          active
            ? 'border-primary/25 bg-primary/10 text-primary'
            : 'border-border-subtle bg-surface-raised text-muted-foreground',
        )}
      >
        <Icon className="size-4" aria-hidden="true" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-[12px] font-medium text-foreground">{label}</span>
        <span className="mt-0.5 block text-[10px] leading-relaxed text-muted-foreground">
          {description}
        </span>
      </span>
    </span>
  )
}
