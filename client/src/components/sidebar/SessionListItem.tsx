import { Check, LayoutGrid, Rows3, User } from 'lucide-react'
import { SESSION_ICONS, getSessionIcon } from './sessionIcons'
import {
  effectiveAttachedClients,
  isSessionAttachedWithLocalTab,
} from './sessionAttachment'
import { formatRelativeTime } from './sessionTime'
import type { Session } from '../../types'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useDateFormat } from '@/hooks/useDateFormat'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { cn } from '@/lib/utils'

type SessionListItemProps = {
  session: Session
  isActive: boolean
  onAttach: (session: string) => void
  onRename: (session: string) => void
  onDetach: (session: string) => void
  onKill: (session: string) => void
  onChangeIcon: (session: string, icon: string) => void
  canDetach: boolean
}

export default function SessionListItem({
  session,
  isActive,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
  canDetach,
}: SessionListItemProps) {
  const { formatTimestamp } = useDateFormat()
  const isAttached = isSessionAttachedWithLocalTab(session, canDetach)
  const attachedClients = effectiveAttachedClients(session.attached, canDetach)
  const unreadPanes = session.unreadPanes ?? 0
  const unreadWindows = session.unreadWindows ?? 0
  const hasUnreadActivity = unreadPanes > 0 || unreadWindows > 0
  const activityRelative = formatRelativeTime(session.activityAt)
  const activityAbsolute = formatTimestamp(session.activityAt)
  const createdAbsolute = formatTimestamp(session.createdAt)

  const SessionIcon = getSessionIcon(session.icon)

  const shortHash =
    session.hash.length > 7
      ? session.hash.slice(0, 3) + '\u2026' + session.hash.slice(-3)
      : session.hash

  const handleOpen = () => {
    onAttach(session.name)
  }

  return (
    <li className="min-w-0">
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <button
            className={cn(
              'group w-full max-w-full cursor-pointer overflow-hidden rounded-lg border bg-surface-elevated px-2.5 py-2 text-left outline-none transition-colors',
              isActive
                ? 'border-primary/60 bg-surface-active-primary shadow-[inset_0_0_0_1px_rgba(59,130,246,.25)]'
                : 'border-border-subtle hover:border-border hover:bg-secondary focus-within:border-border',
            )}
            type="button"
            onClick={handleOpen}
          >
            {/* Line 1: Icon + Name + Hash + Activity */}
            <div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
              <SessionIcon
                className={cn(
                  'h-3.5 w-3.5 shrink-0',
                  isAttached ? 'text-primary-text' : 'text-muted-foreground',
                )}
              />
              <TooltipHelper content={`Created: ${createdAbsolute}`}>
                <span className="min-w-0 flex-1 truncate text-[12px] font-semibold">
                  {session.name}
                </span>
              </TooltipHelper>
              <TooltipHelper content="Windows">
                <span
                  className={cn(
                    'inline-flex h-4 min-w-4 items-center justify-center gap-0.5 rounded-full border bg-surface-overlay px-1 text-[10px]',
                    isAttached && hasUnreadActivity
                      ? 'border-amber-500/50 bg-amber-500/15 text-amber-200'
                      : 'border-border-subtle text-secondary-foreground',
                  )}
                  aria-label={
                    session.windows === 1
                      ? '1 window'
                      : `${session.windows} windows`
                  }
                >
                  <LayoutGrid className="h-2.5 w-2.5" />
                  {session.windows}
                </span>
              </TooltipHelper>
              <TooltipHelper content="Panes">
                <span
                  className="inline-flex h-4 min-w-4 items-center justify-center gap-0.5 rounded-full border border-border-subtle bg-surface-overlay px-1 text-[10px] text-secondary-foreground"
                  aria-label={
                    session.panes === 1 ? '1 pane' : `${session.panes} panes`
                  }
                >
                  <Rows3 className="h-2.5 w-2.5" />
                  {session.panes}
                </span>
              </TooltipHelper>
              {isAttached && (
                <TooltipHelper content="Attached clients">
                  <span
                    className="inline-flex h-4 min-w-4 items-center justify-center gap-0.5 rounded-full border border-primary/40 bg-primary/15 px-1 text-[10px] text-primary-text"
                    aria-label={
                      attachedClients === 1
                        ? '1 client attached'
                        : `${attachedClients} clients attached`
                    }
                  >
                    <User className="h-2.5 w-2.5" />
                    {attachedClients}
                  </span>
                </TooltipHelper>
              )}
            </div>

            {/* Line 2: Content preview (2 lines max, reserved height) */}
            <div
              className={cn(
                'my-1 line-clamp-2 min-h-[2lh] max-w-full overflow-hidden break-all [overflow-wrap:anywhere] text-[10px] leading-[1.4] italic',
                isAttached && hasUnreadActivity
                  ? 'text-secondary-foreground'
                  : 'text-muted-foreground',
              )}
            >
              {session.lastContent || '\u00A0'}
            </div>

            {/* Line 3: hash — windows/panes — time */}
            <div className="mt-1 flex items-center justify-between">
              {session.hash && (
                <TooltipHelper content={`Hash: ${session.hash}`}>
                  <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                    {shortHash}
                  </span>
                </TooltipHelper>
              )}
              <TooltipHelper content={`Last activity: ${activityAbsolute}`}>
                <time
                  className="shrink-0 tabular-nums text-[10px] text-muted-foreground"
                  dateTime={session.activityAt}
                >
                  {activityRelative}
                </time>
              </TooltipHelper>
            </div>
          </button>
        </ContextMenuTrigger>
        <ContextMenuContent className="w-44">
          <ContextMenuItem onSelect={() => onRename(session.name)}>
            Rename Session
          </ContextMenuItem>
          <ContextMenuSub>
            <ContextMenuSubTrigger>Change Icon</ContextMenuSubTrigger>
            <ContextMenuSubContent className="w-36">
              {SESSION_ICONS.map((entry) => {
                const Icon = entry.icon
                const isCurrent =
                  session.icon === entry.key ||
                  (!session.icon && entry.key === 'terminal')
                return (
                  <ContextMenuItem
                    key={entry.key}
                    onSelect={() => onChangeIcon(session.name, entry.key)}
                    className="flex items-center gap-2"
                  >
                    <Icon className="h-3.5 w-3.5" />
                    <span className="flex-1">{entry.label}</span>
                    {isCurrent && <Check className="h-3 w-3 opacity-60" />}
                  </ContextMenuItem>
                )
              })}
            </ContextMenuSubContent>
          </ContextMenuSub>
          <ContextMenuItem
            disabled={!canDetach}
            onSelect={() => onDetach(session.name)}
          >
            Detach Session
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem
            className="text-destructive-foreground focus:text-destructive-foreground"
            onSelect={() => onKill(session.name)}
          >
            Kill Session
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </li>
  )
}
