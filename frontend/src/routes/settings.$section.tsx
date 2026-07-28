import { createFileRoute, redirect } from '@tanstack/react-router'

import DiagnosticsSettings from '@/components/settings/DiagnosticsSettings'
import ExperienceSettings from '@/components/settings/ExperienceSettings'
import IntegrationsSettings from '@/components/settings/IntegrationsSettings'
import OperationsSettings from '@/components/settings/OperationsSettings'
import StorageSettings from '@/components/settings/StorageSettings'
import { isSettingsSection } from '@/components/settings/settingsSections'

export const Route = createFileRoute('/settings/$section')({
  beforeLoad: ({ params }) => {
    guardSettingsSection(params.section)
  },
  component: SettingsSectionRoute,
})

export function guardSettingsSection(section: string): void {
  if (isSettingsSection(section)) return
  throw redirect({
    to: '/settings/$section',
    params: { section: 'experience' },
  })
}

function SettingsSectionRoute() {
  const { section } = Route.useParams()
  switch (section) {
    case 'experience':
      return <ExperienceSettings />
    case 'operations':
      return <OperationsSettings />
    case 'integrations':
      return <IntegrationsSettings />
    case 'storage':
      return <StorageSettings />
    case 'diagnostics':
      return <DiagnosticsSettings />
    default:
      return null
  }
}
