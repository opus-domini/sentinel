import { useState } from 'react'
import { CircleHelp, Lock, LockOpen } from 'lucide-react'
import MetricsHelpDialog from '@/components/MetricsHelpDialog'
import RunbooksHelpDialog from '@/components/RunbooksHelpDialog'
import ServicesHelpDialog from '@/components/ServicesHelpDialog'
import TmuxHelpDialog from '@/components/TmuxHelpDialog'
import { TooltipHelper } from '@/components/TooltipHelper'
import { Button } from '@/components/ui/button'
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
  return (
    <>
      {mobile ? (
        <>
          {helpAction}
          {tokenAction}
        </>
      ) : (
        <>
          {tokenAction}
          {helpAction}
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
      return (
        <TooltipHelper content="Help is unavailable on this page" side="right">
          <span className="block w-full">
            <Button
              variant="ghost"
              size="icon-lg"
              className={cn(triggerClassName, 'cursor-not-allowed')}
              disabled
              aria-label="Help"
              aria-description="Help is unavailable on this page."
            >
              <CircleHelp className="size-4" aria-hidden="true" />
            </Button>
          </span>
        </TooltipHelper>
      )
  }
}
