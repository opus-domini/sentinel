import SidebarShell from './sidebar/SidebarShell'
import SessionControls from './sidebar/SessionControls'
import SessionListPanel from './sidebar/SessionListPanel'
import type { Session } from '../types'

type SessionSidebarProps = {
  sessions: Array<Session>
  totalSessions: number
  openTabs: Array<string>
  activeSession: string
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  defaultCwd: string
  filter: string
  token: string
  tmuxUnavailable: boolean
  recoveryKilledCount: number
  onFilterChange: (value: string) => void
  onTokenChange: (value: string) => void
  onCreate: (name: string, cwd: string) => void
  onOpenRecovery: () => void
  onAttach: (session: string) => void
  onRename: (session: string) => void
  onDetach: (session: string) => void
  onKill: (session: string) => void
  onChangeIcon: (session: string, icon: string) => void
}

export default function SessionSidebar({
  sessions,
  totalSessions,
  openTabs,
  activeSession,
  isOpen,
  collapsed,
  tokenRequired,
  defaultCwd,
  filter,
  token,
  tmuxUnavailable,
  recoveryKilledCount,
  onFilterChange,
  onTokenChange,
  onCreate,
  onOpenRecovery,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
}: SessionSidebarProps) {
  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <SessionControls
          sessionCount={totalSessions}
          tokenRequired={tokenRequired}
          defaultCwd={defaultCwd}
          filter={filter}
          token={token}
          tmuxUnavailable={tmuxUnavailable}
          recoveryKilledCount={recoveryKilledCount}
          onFilterChange={onFilterChange}
          onTokenChange={onTokenChange}
          onCreate={onCreate}
          onOpenRecovery={onOpenRecovery}
        />

        <div className="min-h-0 flex-1">
          <SessionListPanel
            sessions={sessions}
            tmuxUnavailable={tmuxUnavailable}
            openTabs={openTabs}
            activeSession={activeSession}
            filter={filter}
            onFilterChange={onFilterChange}
            onAttach={onAttach}
            onRename={onRename}
            onDetach={onDetach}
            onKill={onKill}
            onChangeIcon={onChangeIcon}
          />
        </div>
      </div>
    </SidebarShell>
  )
}
