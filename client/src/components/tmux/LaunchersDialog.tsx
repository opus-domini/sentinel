import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  DEFAULT_ICON_KEY,
  SESSION_ICONS,
  getSessionIcon,
} from '@/components/sidebar/sessionIcons'
import type { LauncherCwdMode, TmuxLauncher } from '@/types'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { hapticFeedback } from '@/lib/device'
import { cn } from '@/lib/utils'

export type LauncherDraft = {
  id?: string
  name: string
  icon: string
  command: string
  cwdMode: LauncherCwdMode
  cwdValue: string
  windowName: string
}

type LaunchersDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  launchers: Array<TmuxLauncher>
  onSave: (draft: LauncherDraft) => Promise<string | null>
  onDelete: (id: string) => Promise<boolean>
  onReorder: (activeID: string, overID: string) => void
}

const DEFAULT_DRAFT: LauncherDraft = {
  name: '',
  icon: 'terminal',
  command: '',
  cwdMode: 'session',
  cwdValue: '',
  windowName: '',
}

const QUICK_STARTS: Array<{
  name: string
  icon: string
  command: string
  cwdMode: LauncherCwdMode
  windowName: string
}> = [
  {
    name: 'Codex',
    icon: 'code',
    command: 'codex',
    cwdMode: 'active-pane',
    windowName: 'codex',
  },
  {
    name: 'Claude Code',
    icon: 'bot',
    command: 'claude',
    cwdMode: 'active-pane',
    windowName: 'claude',
  },
]

function draftFromLauncher(launcher: TmuxLauncher): LauncherDraft {
  return {
    id: launcher.id,
    name: launcher.name,
    icon: launcher.icon,
    command: launcher.command,
    cwdMode: launcher.cwdMode,
    cwdValue: launcher.cwdValue,
    windowName: launcher.windowName,
  }
}

function SortableLauncherItem({
  launcher,
  selected,
  onSelect,
}: {
  launcher: TmuxLauncher
  selected: boolean
  onSelect: (id: string) => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: launcher.id })
  const Icon = getSessionIcon(launcher.icon)

  return (
    <li
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : undefined,
        zIndex: isDragging ? 10 : undefined,
      }}
    >
      <button
        type="button"
        className={cn(
          'flex w-full cursor-pointer items-center gap-2 rounded-md border px-2 py-2 text-left transition-colors',
          selected
            ? 'border-primary/60 bg-surface-active-primary'
            : 'border-transparent hover:border-border-subtle hover:bg-surface-hover',
        )}
        onClick={() => onSelect(launcher.id)}
        {...attributes}
        {...listeners}
      >
        <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12px] font-semibold">
            {launcher.name}
          </span>
          <span className="block truncate text-[10px] text-muted-foreground">
            {launcher.command}
          </span>
        </span>
        {!launcher.lastUsedAt && (
          <span className="text-[10px] text-muted-foreground">New</span>
        )}
      </button>
    </li>
  )
}

export default function LaunchersDialog({
  open,
  onOpenChange,
  launchers,
  onSave,
  onDelete,
  onReorder,
}: LaunchersDialogProps) {
  const [selectedID, setSelectedID] = useState<string>('new')
  const [draft, setDraft] = useState<LauncherDraft>(DEFAULT_DRAFT)
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
  )

  const startNewLauncher = () => {
    setSelectedID('new')
    setDraft(DEFAULT_DRAFT)
  }

  const applyQuickStart = (preset: (typeof QUICK_STARTS)[number]) => {
    setSelectedID('new')
    setDraft({ ...DEFAULT_DRAFT, ...preset })
  }

  const selectedLauncher = useMemo(
    () => launchers.find((launcher) => launcher.id === selectedID) ?? null,
    [launchers, selectedID],
  )
  const selectedIconEntry = useMemo(
    () =>
      SESSION_ICONS.find((entry) => entry.key === draft.icon) ??
      SESSION_ICONS.find((entry) => entry.key === DEFAULT_ICON_KEY) ??
      SESSION_ICONS[0],
    [draft.icon],
  )

  useEffect(() => {
    if (!open) {
      setSelectedID('new')
      setDraft(DEFAULT_DRAFT)
      return
    }
    if (selectedID === 'new') {
      return
    }
    if (selectedLauncher === null) {
      setSelectedID(launchers[0]?.id ?? 'new')
    }
  }, [launchers, open, selectedID, selectedLauncher])

  useEffect(() => {
    if (!open) return
    if (selectedLauncher !== null) {
      setDraft(draftFromLauncher(selectedLauncher))
      return
    }
    if (selectedID === 'new') {
      setDraft(DEFAULT_DRAFT)
    }
  }, [open, selectedID, selectedLauncher])

  const handleSave = async () => {
    const nextID = await onSave(draft)
    if (nextID) {
      setSelectedID(nextID)
    }
  }

  const handleDelete = async () => {
    if (!draft.id) return
    const deleted = await onDelete(draft.id)
    if (deleted) {
      setSelectedID(launchers.find((item) => item.id !== draft.id)?.id ?? 'new')
      setDraft(DEFAULT_DRAFT)
    }
  }
  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    hapticFeedback()
    onReorder(String(active.id), String(over.id))
  }
  const SelectedIcon = selectedIconEntry.icon

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid min-h-[32rem] max-h-[88vh] gap-4 overflow-hidden sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Launchers</DialogTitle>
          <DialogDescription>
            Configure reusable tmux window launchers for Codex, Claude Code, and
            any other command workflow.
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 gap-4 md:grid-cols-[15rem_minmax(0,1fr)]">
          <section className="grid min-h-0 content-start gap-3">
            <div className="flex items-center">
              <Button
                type="button"
                variant="outline"
                className="flex-1 cursor-pointer justify-start rounded-r-none border-r-0"
                onClick={startNewLauncher}
              >
                New launcher
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="default"
                    className="w-7 cursor-pointer rounded-l-none px-1.5"
                    aria-label="Open launcher presets"
                  >
                    <ChevronDown className="h-3.5 w-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuItem
                    className="cursor-pointer"
                    onSelect={startNewLauncher}
                  >
                    Blank launcher
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>Starter presets</DropdownMenuLabel>
                  {QUICK_STARTS.map((preset) => {
                    const Icon = getSessionIcon(preset.icon)
                    return (
                      <DropdownMenuItem
                        key={preset.name}
                        className="cursor-pointer"
                        onSelect={() => applyQuickStart(preset)}
                      >
                        <Icon className="h-3.5 w-3.5" />
                        {preset.name}
                      </DropdownMenuItem>
                    )
                  })}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {launchers.length === 0 ? (
              <EmptyState
                variant="inline"
                className="grid gap-2 p-3 text-left text-[12px]"
              >
                <span className="text-[12px]">
                  No launchers configured yet.
                </span>
                <span className="text-muted-foreground">
                  Start from a blank launcher or pick a preset from the split
                  button above.
                </span>
              </EmptyState>
            ) : (
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={launchers.map((launcher) => launcher.id)}
                  strategy={verticalListSortingStrategy}
                >
                  <ul className="grid min-h-0 content-start list-none gap-1 overflow-y-auto rounded-lg border border-border-subtle bg-secondary p-2">
                    {launchers.map((launcher) => (
                      <SortableLauncherItem
                        key={launcher.id}
                        launcher={launcher}
                        selected={launcher.id === selectedID}
                        onSelect={setSelectedID}
                      />
                    ))}
                  </ul>
                </SortableContext>
              </DndContext>
            )}
          </section>

          <section className="grid min-h-0 gap-3 overflow-hidden rounded-lg border border-border-subtle bg-secondary p-3">
            <div className="grid gap-2 md:grid-cols-2">
              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                Name
                <Input
                  className="bg-surface-overlay"
                  value={draft.name}
                  onChange={(event) =>
                    setDraft((prev) => ({ ...prev, name: event.target.value }))
                  }
                  placeholder="Codex"
                />
              </label>

              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                Icon
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      className="w-full cursor-pointer justify-start bg-surface-overlay text-[12px]"
                    >
                      <SelectedIcon className="h-3.5 w-3.5 text-muted-foreground" />
                      {selectedIconEntry.label}
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="z-[60]">
                    {SESSION_ICONS.map((entry) => {
                      const Icon = entry.icon
                      return (
                        <DropdownMenuItem
                          key={entry.key}
                          className="cursor-pointer"
                          onSelect={() =>
                            setDraft((prev) => ({ ...prev, icon: entry.key }))
                          }
                        >
                          <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                          {entry.label}
                        </DropdownMenuItem>
                      )
                    })}
                  </DropdownMenuContent>
                </DropdownMenu>
              </label>
            </div>

            <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
              Command
              <Input
                className="bg-surface-overlay font-mono"
                value={draft.command}
                onChange={(event) =>
                  setDraft((prev) => ({ ...prev, command: event.target.value }))
                }
                placeholder="codex"
              />
            </label>

            <div className="grid gap-2 md:grid-cols-2">
              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                Working Directory
                <Select
                  value={draft.cwdMode}
                  onValueChange={(value: LauncherCwdMode) =>
                    setDraft((prev) => ({
                      ...prev,
                      cwdMode: value,
                      cwdValue: value === 'fixed' ? prev.cwdValue : '',
                    }))
                  }
                >
                  <SelectTrigger className="w-full cursor-pointer bg-surface-overlay text-[12px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="z-[60]">
                    <SelectItem value="session" className="cursor-pointer">
                      session cwd
                    </SelectItem>
                    <SelectItem value="active-pane" className="cursor-pointer">
                      active pane cwd
                    </SelectItem>
                    <SelectItem value="fixed" className="cursor-pointer">
                      fixed path
                    </SelectItem>
                  </SelectContent>
                </Select>
              </label>

              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                Window Name
                <Input
                  className="bg-surface-overlay"
                  value={draft.windowName}
                  onChange={(event) =>
                    setDraft((prev) => ({
                      ...prev,
                      windowName: event.target.value,
                    }))
                  }
                  placeholder="codex"
                />
              </label>
            </div>

            {draft.cwdMode === 'fixed' && (
              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                Fixed Path
                <Input
                  className="bg-surface-overlay font-mono"
                  value={draft.cwdValue}
                  onChange={(event) =>
                    setDraft((prev) => ({
                      ...prev,
                      cwdValue: event.target.value,
                    }))
                  }
                  placeholder="/home/hugo/project"
                />
              </label>
            )}

            <div className="rounded-md border border-border-subtle bg-surface-overlay px-3 py-2 text-[11px] text-muted-foreground">
              Launchers always open a new tmux window from the active session.
              The `+` menu becomes the fast path to use them.
            </div>

            <div className="mt-auto flex flex-wrap items-center gap-2">
              <div className="ml-auto flex items-center gap-2">
                {draft.id && (
                  <Button
                    type="button"
                    variant="destructive"
                    size="sm"
                    className="cursor-pointer"
                    onClick={handleDelete}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    Delete
                  </Button>
                )}
                <Button
                  type="button"
                  size="sm"
                  className="cursor-pointer"
                  onClick={handleSave}
                >
                  Save
                </Button>
              </div>
            </div>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  )
}
