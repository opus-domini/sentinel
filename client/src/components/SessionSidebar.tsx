import { useMemo } from 'react'
import SidebarShell from './sidebar/SidebarShell'
import PinnedSessionsPanel from './sidebar/PinnedSessionsPanel'
import SessionControls from './sidebar/SessionControls'
import SessionListPanel from './sidebar/SessionListPanel'
import type { Session, SessionPreset } from '../types'

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
  onCreate: (name: string, cwd: string) => void
  onPinSession: (session: string) => void
  onUnpinSession: (name: string) => void
  onLaunchPreset: (name: string) => void
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
  onPinSession,
  onUnpinSession,
  onLaunchPreset,
  onReorderPinned,
  onReorderSession,
  onAttach,
  onRename,
  onDetach,
  onKill,
  onChangeIcon,
}: SessionSidebarProps) {
  const pinnedNames = useMemo(
    () => new Set(presets.map((preset) => preset.name)),
    [presets],
  )
  const hasVisibleRegularSessions = useMemo(
    () => sessions.some((session) => !pinnedNames.has(session.name)),
    [pinnedNames, sessions],
  )
  const shouldHideRegularPanel =
    filter.trim() === '' &&
    !tmuxUnavailable &&
    sessions.length > 0 &&
    presets.length > 0 &&
    !hasVisibleRegularSessions

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="flex h-full min-h-0 flex-col gap-2">
        <SessionControls
          sessionCount={totalSessions}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          defaultCwd={defaultCwd}
          filter={filter}
          tmuxUnavailable={tmuxUnavailable}
          onFilterChange={onFilterChange}
          onTokenChange={onTokenChange}
          onCreate={onCreate}
        />

        <div className={shouldHideRegularPanel ? 'min-h-0 flex-1' : ''}>
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
            fillHeight={shouldHideRegularPanel}
          />
        </div>

        {!shouldHideRegularPanel && (
          <div className="min-h-0 flex-1">
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
            />
          </div>
        )}
      </div>
    </SidebarShell>
  )
}
