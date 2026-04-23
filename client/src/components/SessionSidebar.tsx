import SidebarShell from './sidebar/SidebarShell'
import PinnedSessionsPanel from './sidebar/PinnedSessionsPanel'
import SessionControls from './sidebar/SessionControls'
import SessionListPanel from './sidebar/SessionListPanel'
import type { Session, SessionPreset } from '../types'
import { useLayoutContext } from '@/contexts/LayoutContext'

type SessionSidebarProps = {
  sessions: Array<Session>
  totalSessions: number
  openTabs: Array<string>
  activeSession: string
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  authenticated: boolean
  defaultCwd: string
  presets: Array<SessionPreset>
  filter: string
  tmuxUnavailable: boolean
  onFilterChange: (value: string) => void
  onTokenChange: (value: string) => void
  onCreate: (name: string, cwd: string, user?: string) => void
  onSavePreset: (input: {
    previousName: string
    name: string
    cwd: string
    icon: string
    user: string
  }) => Promise<boolean>
  onPinSession: (session: string) => void
  onUnpinSession: (name: string) => Promise<boolean>
  onLaunchPreset: (name: string) => void
  onReorderPresets: (activeName: string, overName: string) => void
  onReorderPinned: (activeName: string, overName: string) => void
  onReorderSession: (activeName: string, overName: string) => void
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
  authenticated,
  defaultCwd,
  presets,
  filter,
  tmuxUnavailable,
  onFilterChange,
  onTokenChange,
  onCreate,
  onSavePreset,
  onPinSession,
  onUnpinSession,
  onLaunchPreset,
  onReorderPresets,
  onReorderPinned,
  onReorderSession,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
}: SessionSidebarProps) {
  const { sidebarDensity } = useLayoutContext()
  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto overscroll-contain no-scrollbar">
        <SessionControls
          sessionCount={totalSessions}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          defaultCwd={defaultCwd}
          presets={presets}
          filter={filter}
          tmuxUnavailable={tmuxUnavailable}
          onFilterChange={onFilterChange}
          onTokenChange={onTokenChange}
          onCreate={onCreate}
          onSavePreset={onSavePreset}
          onLaunchPreset={onLaunchPreset}
          onDeletePreset={onUnpinSession}
          onReorderPresets={onReorderPresets}
        />

        <PinnedSessionsPanel
          sessions={sessions}
          presets={presets}
          filter={filter}
          openTabs={openTabs}
          activeSession={activeSession}
          tmuxUnavailable={tmuxUnavailable}
          onAttach={onAttach}
          onRename={onRename}
          onDetach={onDetach}
          onKill={onKill}
          onChangeIcon={onChangeIcon}
          onPinSession={onPinSession}
          onUnpinSession={onUnpinSession}
          onLaunchPreset={onLaunchPreset}
          onReorder={onReorderPinned}
          density={sidebarDensity}
        />

        <SessionListPanel
          sessions={sessions}
          tmuxUnavailable={tmuxUnavailable}
          openTabs={openTabs}
          activeSession={activeSession}
          filter={filter}
          presets={presets}
          onFilterChange={onFilterChange}
          onAttach={onAttach}
          onRename={onRename}
          onDetach={onDetach}
          onKill={onKill}
          onChangeIcon={onChangeIcon}
          onPinSession={onPinSession}
          onUnpinSession={onUnpinSession}
          onReorder={onReorderSession}
          density={sidebarDensity}
        />
      </div>
    </SidebarShell>
  )
}
