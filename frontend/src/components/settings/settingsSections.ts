import { Database, Plug, Settings2, Wrench } from 'lucide-react'

export const SETTINGS_SECTIONS = [
  {
    id: 'experience',
    label: 'Experience',
    description: 'Theme, timezone, and date format',
    Icon: Settings2,
  },
  {
    id: 'integrations',
    label: 'Integrations',
    description: 'Connections to external tools',
    Icon: Plug,
  },
  {
    id: 'storage',
    label: 'Storage',
    description: 'Usage and protected history',
    Icon: Database,
  },
  {
    id: 'diagnostics',
    label: 'Diagnostics',
    description: 'Runtime, deployment, and app',
    Icon: Wrench,
  },
] as const

export type SettingsSection = (typeof SETTINGS_SECTIONS)[number]['id']

export function isSettingsSection(value: string): value is SettingsSection {
  return SETTINGS_SECTIONS.some((section) => section.id === value)
}

export function settingsSectionFromPath(pathname: string): SettingsSection {
  const candidate = pathname.split('/').filter(Boolean).at(-1) ?? ''
  return isSettingsSection(candidate) ? candidate : 'experience'
}
