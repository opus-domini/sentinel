import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowLeft, ChevronDown, ChevronUp, List, ListTree } from 'lucide-react'
import type { SystemTerminalDetailResponse, TerminalProcess } from '@/types'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipHelper } from '@/components/TooltipHelper'
import { useTokenContext } from '@/contexts/TokenContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { cn } from '@/lib/utils'

type SystemTerminalDetailProps = {
  tty: string
  onBack: () => void
}

type SortKey = 'pid' | 'user' | 'cpu' | 'mem' | 'command'
type SortDir = 'asc' | 'desc'

type TreeNode = {
  process: TerminalProcess
  children: Array<TreeNode>
  depth: number
  /** Whether this node is the last child of its parent at each depth */
  ancestorIsLast: Array<boolean>
}

function buildTree(processes: Array<TerminalProcess>): Array<TreeNode> {
  const pidSet = new Set(processes.map((p) => p.pid))
  const childrenMap = new Map<number, Array<TerminalProcess>>()

  for (const proc of processes) {
    const parentPid = pidSet.has(proc.ppid) ? proc.ppid : -1
    const list = childrenMap.get(parentPid)
    if (list) {
      list.push(proc)
    } else {
      childrenMap.set(parentPid, [proc])
    }
  }

  function build(
    parentPid: number,
    depth: number,
    ancestorIsLast: Array<boolean>,
  ): Array<TreeNode> {
    const children = childrenMap.get(parentPid) ?? []
    return children.map((proc, i) => {
      const isLast = i === children.length - 1
      const trail = [...ancestorIsLast, isLast]
      return {
        process: proc,
        children: build(proc.pid, depth + 1, trail),
        depth,
        ancestorIsLast: trail,
      }
    })
  }

  return build(-1, 0, [])
}

function flattenTree(nodes: Array<TreeNode>): Array<TreeNode> {
  const result: Array<TreeNode> = []
  function walk(list: Array<TreeNode>) {
    for (const node of list) {
      result.push(node)
      walk(node.children)
    }
  }
  walk(nodes)
  return result
}

function compareProcesses(
  a: TerminalProcess,
  b: TerminalProcess,
  key: SortKey,
  mul: number,
): number {
  switch (key) {
    case 'pid':
      return (a.pid - b.pid) * mul
    case 'user':
      return a.user.localeCompare(b.user) * mul
    case 'cpu':
      return (a.cpu - b.cpu) * mul
    case 'mem':
      return (a.mem - b.mem) * mul
    case 'command':
      return a.args.localeCompare(b.args) * mul
  }
}

function sortFlat(
  processes: Array<TerminalProcess>,
  key: SortKey,
  dir: SortDir,
): Array<TerminalProcess> {
  const mul = dir === 'asc' ? 1 : -1
  return [...processes].sort((a, b) => compareProcesses(a, b, key, mul))
}

function sortTreeSiblings(
  nodes: Array<TreeNode>,
  key: SortKey,
  dir: SortDir,
  parentTrail: Array<boolean>,
): Array<TreeNode> {
  const mul = dir === 'asc' ? 1 : -1
  const sorted = [...nodes].sort((a, b) =>
    compareProcesses(a.process, b.process, key, mul),
  )
  return sorted.map((node, i) => {
    const isLast = i === sorted.length - 1
    const trail = [...parentTrail, isLast]
    return {
      ...node,
      ancestorIsLast: trail,
      children: sortTreeSiblings(node.children, key, dir, trail),
    }
  })
}

function TreePrefix({ trail }: { trail: Array<boolean> }) {
  if (trail.length === 0) return null
  const parts: Array<string> = []
  for (let i = 0; i < trail.length; i++) {
    if (i < trail.length - 1) {
      parts.push(trail[i] ? '   ' : '│  ')
    } else {
      parts.push(trail[i] ? '└─ ' : '├─ ')
    }
  }
  return <span className="text-muted-foreground/50">{parts.join('')}</span>
}

function SortIcon({ active, dir }: { active: boolean; dir: SortDir }) {
  if (!active) return null
  return dir === 'asc' ? (
    <ChevronUp className="inline h-3 w-3" />
  ) : (
    <ChevronDown className="inline h-3 w-3" />
  )
}

const GRID_COLS = 'grid-cols-[56px_72px_52px_52px_1fr]'

export default function SystemTerminalDetail({
  tty,
  onBack,
}: SystemTerminalDetailProps) {
  const { token } = useTokenContext()
  const api = useTmuxApi(token)

  const [processes, setProcesses] = useState<Array<TerminalProcess>>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [viewMode, setViewMode] = useState<'tree' | 'flat'>('tree')
  const [sortKey, setSortKey] = useState<SortKey>('pid')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  const fetchProcesses = useCallback(
    async (background = false) => {
      if (!background) setLoading(true)
      try {
        const data = await api<SystemTerminalDetailResponse>(
          `/api/terminals/system/${encodeURIComponent(tty)}`,
        )
        setProcesses(data.processes)
        setError('')
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'failed to load processes',
        )
      } finally {
        if (!background) setLoading(false)
      }
    },
    [api, tty],
  )

  useEffect(() => {
    void fetchProcesses()
    const id = window.setInterval(() => {
      void fetchProcesses(true)
    }, 4_000)
    return () => {
      window.clearInterval(id)
    }
  }, [fetchProcesses])

  const toggleSort = useCallback((key: SortKey) => {
    setSortKey((prev) => {
      if (prev === key) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return key
      }
      setSortDir(key === 'cpu' || key === 'mem' ? 'desc' : 'asc')
      return key
    })
  }, [])

  const flatRows = useMemo(() => {
    if (viewMode !== 'flat') return []
    return sortFlat(processes, sortKey, sortDir)
  }, [processes, sortKey, sortDir, viewMode])

  const treeRows = useMemo(() => {
    if (viewMode !== 'tree') return []
    const tree = buildTree(processes)
    const sorted = sortTreeSiblings(tree, sortKey, sortDir, [])
    return flattenTree(sorted)
  }, [processes, sortKey, sortDir, viewMode])

  const user = processes.length > 0 ? processes[0].user : ''

  const headerButton = (key: SortKey, label: string, className?: string) => (
    <button
      type="button"
      className={cn(
        'flex cursor-pointer select-none items-center gap-0.5 hover:text-foreground',
        className,
      )}
      onClick={() => toggleSort(key)}
    >
      {label}
      <SortIcon active={sortKey === key} dir={sortDir} />
    </button>
  )

  const renderRow = (proc: TerminalProcess, commandPrefix?: JSX.Element) => (
    <>
      <span className="text-right font-mono text-muted-foreground">
        {proc.pid}
      </span>
      <span className="min-w-0 truncate text-muted-foreground">
        {proc.user}
      </span>
      <span className="text-right font-mono text-muted-foreground">
        {proc.cpu.toFixed(1)}
      </span>
      <span className="text-right font-mono text-muted-foreground">
        {proc.mem.toFixed(1)}
      </span>
      <span
        className="min-w-0 truncate font-mono text-foreground"
        title={proc.args}
      >
        {commandPrefix}
        {proc.args}
      </span>
    </>
  )

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between gap-2 border-b border-border-subtle px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 rounded border border-primary/45 bg-primary/15 px-1.5 py-0.5 text-[11px] font-semibold text-primary-text">
            {tty}
          </span>
          {user && (
            <span className="truncate text-[12px] text-muted-foreground">
              {user}
            </span>
          )}
          <span className="text-[11px] text-muted-foreground">
            {processes.length} process{processes.length !== 1 ? 'es' : ''}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <TooltipHelper
            content={viewMode === 'tree' ? 'Flat view' : 'Tree view'}
          >
            <Button
              variant="ghost"
              size="icon-xs"
              className="text-muted-foreground hover:text-foreground"
              onClick={() =>
                setViewMode((v) => (v === 'tree' ? 'flat' : 'tree'))
              }
              aria-label={
                viewMode === 'tree'
                  ? 'Switch to flat view'
                  : 'Switch to tree view'
              }
            >
              {viewMode === 'tree' ? (
                <List className="h-3.5 w-3.5" />
              ) : (
                <ListTree className="h-3.5 w-3.5" />
              )}
            </Button>
          </TooltipHelper>
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0 gap-1 text-[12px] text-muted-foreground hover:text-foreground"
            onClick={onBack}
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1">
        {loading && processes.length === 0 && (
          <EmptyState className="border-dashed text-[12px]">
            Loading processes...
          </EmptyState>
        )}

        {!loading && error !== '' && (
          <EmptyState className="border-dashed text-[12px] text-destructive-foreground">
            {error}
          </EmptyState>
        )}

        {!loading && error === '' && processes.length === 0 && (
          <EmptyState className="border-dashed text-[12px]">
            No processes found on {tty}.
          </EmptyState>
        )}

        {processes.length > 0 && (
          <ScrollArea className="h-full">
            <div className="grid gap-0.5 p-2">
              <div
                className={cn(
                  'grid gap-2 px-2 py-1 text-[10px] uppercase tracking-[0.06em] text-muted-foreground',
                  GRID_COLS,
                )}
              >
                {headerButton('pid', 'PID', 'justify-end')}
                {headerButton('user', 'USER')}
                {headerButton('cpu', 'CPU%', 'justify-end')}
                {headerButton('mem', 'MEM%', 'justify-end')}
                {headerButton('command', 'COMMAND')}
              </div>

              {viewMode === 'flat' &&
                flatRows.map((proc) => (
                  <div
                    key={proc.pid}
                    className={cn(
                      'grid gap-2 rounded px-2 py-1 text-[12px] hover:bg-surface-elevated',
                      GRID_COLS,
                    )}
                  >
                    {renderRow(proc)}
                  </div>
                ))}

              {viewMode === 'tree' &&
                treeRows.map((node) => (
                  <div
                    key={node.process.pid}
                    className={cn(
                      'grid gap-2 rounded px-2 py-1 text-[12px] hover:bg-surface-elevated',
                      GRID_COLS,
                    )}
                  >
                    {renderRow(
                      node.process,
                      <TreePrefix trail={node.ancestorIsLast} />,
                    )}
                  </div>
                ))}
            </div>
          </ScrollArea>
        )}
      </div>
    </div>
  )
}
