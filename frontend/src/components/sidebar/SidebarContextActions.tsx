import { useState } from 'react'
import { Lock, LockOpen, Settings } from 'lucide-react'
import MetricsHelpDialog from '@/components/MetricsHelpDialog'
import RunbooksHelpDialog from '@/components/RunbooksHelpDialog'
import ServicesHelpDialog from '@/components/ServicesHelpDialog'
import TmuxHelpDialog from '@/components/TmuxHelpDialog'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useTokenContext } from '@/contexts/TokenContext'
import { cn } from '@/lib/utils'
import TokenDialog from './TokenDialog'

type SidebarContextActionsProps = {
  pathname: string
  mobile?: boolean
}

export default function SidebarContextActions({
  pathname,
  mobile = false,
}: SidebarContextActionsProps) {
  const { setSettingsOpen } = useLayoutContext()
  const { tokenRequired } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const [tokenOpen, setTokenOpen] = useState(false)
  const lockLabel = authenticated ? 'Authenticated (required)' : 'Token required'
  const buttonClassName = cn(
    'cursor-pointer text-secondary-foreground hover:text-foreground',
    mobile ? 'size-8 w-8 touch-manipulation' : 'w-full',
  )
  const tokenAction = tokenRequired && (
    <TooltipHelper content={lockLabel} side={mobile ? 'bottom' : 'right'}>
      <Button
        variant="ghost"
        size="icon-lg"
        className={buttonClassName}
        onClick={() => setTokenOpen(true)}
        aria-label="API token"
        aria-description={lockLabel}
      >
        {authenticated ? <Lock className="size-4" /> : <LockOpen className="size-4" />}
      </Button>
    </TooltipHelper>
  )
  const helpAction = <PageHelpDialog pathname={pathname} triggerClassName={buttonClassName} />
  const settingsAction = (
    <TooltipHelper content="Settings" side={mobile ? 'bottom' : 'right'}>
      <Button
        variant="ghost"
        size="icon-lg"
        className={buttonClassName}
        onClick={() => setSettingsOpen(true)}
        aria-label="Settings"
      >
        <Settings className="size-4" />
      </Button>
    </TooltipHelper>
  )

  return (
    <>
      {mobile ? (
        <>
          {settingsAction}
          {helpAction}
          {tokenAction}
        </>
      ) : (
        <>
          {tokenAction}
          {helpAction}
          {settingsAction}
        </>
      )}
      {tokenRequired && (
        <TokenDialog
          open={tokenOpen}
          onOpenChange={setTokenOpen}
          authenticated={authenticated}
          onTokenChange={setToken}
          tokenRequired
        />
      )}
    </>
  )
}

function PageHelpDialog({
  pathname,
  triggerClassName,
}: {
  pathname: string
  triggerClassName: string
}) {
  switch (pathname) {
    case '/tmux':
      return <TmuxHelpDialog triggerClassName={triggerClassName} />
    case '/runbooks':
      return <RunbooksHelpDialog triggerClassName={triggerClassName} />
    case '/services':
      return <ServicesHelpDialog triggerClassName={triggerClassName} />
    case '/metrics':
      return <MetricsHelpDialog triggerClassName={triggerClassName} />
    default:
      return null
  }
}
