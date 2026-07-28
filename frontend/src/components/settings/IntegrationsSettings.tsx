import { Plug } from 'lucide-react'

import MCPSettingsPanel from './MCPSettingsPanel'
import SettingsSectionHeader from './SettingsSectionHeader'
import { useMetaContext } from '@/contexts/MetaContext'

export default function IntegrationsSettings() {
  const { hostname } = useMetaContext()
  return (
    <div className="grid gap-4">
      <SettingsSectionHeader
        title="Integrations"
        description="Connect trusted tools to Sentinel while keeping credentials outside project files."
        icon={<Plug className="size-4" aria-hidden="true" />}
      />
      <MCPSettingsPanel hostname={hostname} />
    </div>
  )
}
