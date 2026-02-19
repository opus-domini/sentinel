import type { RefCallback } from 'react'
import { EmptyState } from '@/components/ui/empty-state'

type TerminalHostProps = {
  openTabs: Array<string>
  activeSession: string
  getTerminalHostRef: (session: string) => RefCallback<HTMLDivElement>
}

export default function TerminalHost({
  openTabs,
  activeSession,
  getTerminalHostRef,
}: TerminalHostProps) {
  if (openTabs.length === 0) {
    return (
      <EmptyState className="border-dashed text-[12px]">
        Open a session to attach the terminal
      </EmptyState>
    )
  }

  return (
    <>
      {openTabs.map((sessionName) => (
        <div
          key={sessionName}
          ref={getTerminalHostRef(sessionName)}
          className={
            sessionName === activeSession
              ? 'absolute inset-0 min-h-0 overflow-hidden'
              : 'hidden absolute inset-0 min-h-0 overflow-hidden'
          }
        />
      ))}
    </>
  )
}
