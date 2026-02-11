import { useMemo, useState } from 'react'
import { Cpu, X } from 'lucide-react'
import SidebarHeader from './sidebar/SidebarHeader'
import SidebarShell from './sidebar/SidebarShell'
import TokenDialog from './sidebar/TokenDialog'
import type { TerminalConnection } from '../types'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { cn } from '@/lib/utils'

type TerminalsSidebarProps = {
  isOpen: boolean
  collapsed: boolean
  tokenRequired: boolean
  token: string
  sentinelTerminals: Array<string>
  activeTerminal: string
  systemTerminals: Array<TerminalConnection>
  loadingSystemTerminals: boolean
  systemTerminalsError: string
  activeSystemTTY: string | null
  onTokenChange: (value: string) => void
  onCreateTerminal: () => void
  onSelectTerminal: (terminal: string) => void
  onCloseTerminal: (terminal: string) => void
  onSelectSystemTerminal: (tty: string) => void
}

function parseTTY(tty: string): { family: string; index: number } {
  const match = /^([a-zA-Z]+)\/?(\d+)$/.exec(tty)
  if (!match) {
    return { family: tty, index: Number.MAX_SAFE_INTEGER }
  }
  return {
    family: match[1].toLowerCase(),
    index: Number.parseInt(match[2], 10),
  }
}

function compareTTY(leftTTY: string, rightTTY: string): number {
  const left = parseTTY(leftTTY)
  const right = parseTTY(rightTTY)

  if (left.family !== right.family) {
    return left.family.localeCompare(right.family)
  }
  if (left.index !== right.index) {
    return left.index - right.index
  }
  return leftTTY.localeCompare(rightTTY)
}

export default function TerminalsSidebar({
  isOpen,
  collapsed,
  tokenRequired,
  token,
  sentinelTerminals,
  activeTerminal,
  systemTerminals,
  loadingSystemTerminals,
  systemTerminalsError,
  activeSystemTTY,
  onTokenChange,
  onCreateTerminal,
  onSelectTerminal,
  onCloseTerminal,
  onSelectSystemTerminal,
}: TerminalsSidebarProps) {
  const [isTokenOpen, setIsTokenOpen] = useState(false)
  const hasToken = token.trim() !== ''
  const totalTerminals = sentinelTerminals.length + systemTerminals.length

  const lockLabel = useMemo(() => {
    if (tokenRequired) {
      return hasToken ? 'Token configured (required)' : 'Token required'
    }
    return hasToken ? 'Token configured' : 'No token'
  }, [hasToken, tokenRequired])

  const sortedSystemTerminals = useMemo(() => {
    const list = [...systemTerminals]
    list.sort((left, right) => compareTTY(left.tty, right.tty))
    return list
  }, [systemTerminals])

  return (
    <SidebarShell isOpen={isOpen} collapsed={collapsed}>
      <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-2">
        <section className="grid gap-2 rounded-lg border border-border-subtle bg-secondary p-2">
          <SidebarHeader
            title="Terminals"
            count={totalTerminals}
            hasToken={hasToken}
            lockTitle={lockLabel}
            canCreate
            onToggleAdd={onCreateTerminal}
            onToggleLock={() => setIsTokenOpen(true)}
          />

          <TokenDialog
            open={isTokenOpen}
            onOpenChange={setIsTokenOpen}
            token={token}
            onTokenChange={onTokenChange}
            tokenRequired={tokenRequired}
          />
        </section>

        <section className="min-h-0 overflow-hidden rounded-lg border border-border-subtle bg-secondary">
          <ScrollArea className="h-full">
            <div className="grid min-h-0 list-none gap-1.5 p-2">
              <div className="px-1 pt-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                Sentinel
              </div>

              {sentinelTerminals.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  No open sentinel terminals.
                </EmptyState>
              )}

              {sentinelTerminals.map((terminalName) => {
                const isActive = terminalName === activeTerminal
                return (
                  <button
                    key={terminalName}
                    type="button"
                    className={cn(
                      'flex min-w-0 cursor-pointer items-center gap-1.5 rounded border px-2 py-1.5 text-left text-[12px]',
                      isActive
                        ? 'border-primary/45 bg-primary/15 text-primary-text-bright'
                        : 'border-border-subtle bg-surface-elevated text-secondary-foreground hover:bg-secondary hover:text-foreground',
                    )}
                    onClick={() => onSelectTerminal(terminalName)}
                  >
                    <span
                      className="min-w-0 flex-1 truncate"
                      title={terminalName}
                    >
                      {terminalName}
                    </span>
                    <TooltipHelper content={`Close ${terminalName}`}>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="shrink-0 text-muted-foreground hover:text-foreground"
                        onClick={(event) => {
                          event.preventDefault()
                          event.stopPropagation()
                          onCloseTerminal(terminalName)
                        }}
                        aria-label={`Close ${terminalName}`}
                      >
                        <X className="h-3 w-3" />
                      </Button>
                    </TooltipHelper>
                  </button>
                )
              })}

              <div className="px-1 pt-2 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
                System
              </div>

              {loadingSystemTerminals && sortedSystemTerminals.length === 0 && (
                <EmptyState
                  variant="inline"
                  className="border-dashed text-[12px]"
                >
                  Loading system terminals...
                </EmptyState>
              )}

              {!loadingSystemTerminals &&
                sortedSystemTerminals.length === 0 && (
                  <EmptyState
                    variant="inline"
                    className="border-dashed text-[12px]"
                  >
                    No active host terminals found.
                  </EmptyState>
                )}

              {sortedSystemTerminals.map((terminal) => {
                const isActive = terminal.tty === activeSystemTTY
                return (
                  <button
                    key={terminal.id}
                    type="button"
                    className={cn(
                      'flex min-w-0 cursor-pointer items-center gap-1.5 rounded border px-2 py-1.5 text-left text-[12px]',
                      isActive
                        ? 'border-primary/45 bg-primary/15 text-primary-text-bright'
                        : 'border-border-subtle bg-surface-elevated text-secondary-foreground hover:bg-secondary hover:text-foreground',
                    )}
                    onClick={() => onSelectSystemTerminal(terminal.tty)}
                  >
                    <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground" />
                    <span className="shrink-0 text-[12px] font-semibold">
                      {terminal.tty}
                    </span>
                    <span
                      className="min-w-0 flex-1 truncate text-secondary-foreground"
                      title={terminal.command || '-'}
                    >
                      {terminal.command || '-'}
                    </span>
                    <TooltipHelper
                      content={`${terminal.processCount} running process${terminal.processCount !== 1 ? 'es' : ''}`}
                    >
                      <span className="inline-flex shrink-0 items-center gap-0.5 rounded-full border border-border-subtle bg-surface-overlay px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        <Cpu className="h-3 w-3" />
                        {terminal.processCount}
                      </span>
                    </TooltipHelper>
                  </button>
                )
              })}

              {systemTerminalsError !== '' && (
                <p className="mt-1 text-[12px] text-destructive-foreground">
                  {systemTerminalsError}
                </p>
              )}
            </div>
          </ScrollArea>
        </section>
      </div>
    </SidebarShell>
  )
}
