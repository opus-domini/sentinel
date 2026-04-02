import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { DEFAULT_ICON_KEY, TMUX_ICONS, getTmuxIcon } from '@/lib/tmuxIcons'
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
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
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

function describeLauncherCommand(command: string) {
  const normalized = command.trim()
  if (normalized !== '') {
    return normalized
  }
  return 'plain shell'
}

function SortableLauncherItem({
  launcher,
  selected,
  dragEnabled,
  onSelect,
}: {
  launcher: TmuxLauncher
  selected: boolean
  dragEnabled: boolean
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
  const Icon = getTmuxIcon(launcher.icon)

  return (
    <li
      ref={setNodeRef}
      style={{
        transform: dragEnabled ? CSS.Transform.toString(transform) : undefined,
        transition: dragEnabled ? transition : undefined,
        opacity: dragEnabled && isDragging ? 0.5 : undefined,
        zIndex: dragEnabled && isDragging ? 10 : undefined,
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
        style={{ touchAction: dragEnabled ? undefined : 'pan-y' }}
        {...(dragEnabled ? attributes : {})}
        {...(dragEnabled ? listeners : {})}
      >
        <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12px] font-semibold">
            {launcher.name}
          </span>
          <span className="block truncate text-[10px] text-muted-foreground">
            {describeLauncherCommand(launcher.command)}
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
  const isMobile = useIsMobileLayout()
  const dragEnabled = !isMobile
  const [selectedID, setSelectedID] = useState<string>('new')
  const [draft, setDraft] = useState<LauncherDraft>(DEFAULT_DRAFT)
  const [saveError, setSaveError] = useState('')
  const [saving, setSaving] = useState(false)
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  const startNewLauncher = () => {
    setSaveError('')
    setSelectedID('new')
    setDraft(DEFAULT_DRAFT)
  }

  const updateDraft = (updater: (prev: LauncherDraft) => LauncherDraft) => {
    setSaveError('')
    setDraft(updater)
  }

  const selectLauncher = (id: string) => {
    setSaveError('')
    setSelectedID(id)
  }

  const applyQuickStart = (preset: (typeof QUICK_STARTS)[number]) => {
    setSaveError('')
    setSelectedID('new')
    setDraft({ ...DEFAULT_DRAFT, ...preset })
  }

  const selectedLauncher = useMemo(
    () => launchers.find((launcher) => launcher.id === selectedID) ?? null,
    [launchers, selectedID],
  )
  const selectedIconEntry = useMemo(
    () =>
      TMUX_ICONS.find((entry) => entry.key === draft.icon) ??
      TMUX_ICONS.find((entry) => entry.key === DEFAULT_ICON_KEY) ??
      TMUX_ICONS[0],
    [draft.icon],
  )

  useEffect(() => {
    if (!open) {
      setSaveError('')
      setSaving(false)
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
    setSaving(true)
    setSaveError('')
    try {
      const nextID = await onSave(draft)
      if (nextID) {
        setSelectedID(nextID)
      }
    } catch (error) {
      setSaveError(
        error instanceof Error && error.message.trim() !== ''
          ? error.message
          : 'failed to save launcher',
      )
    } finally {
      setSaving(false)
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
      <DialogContent className="inset-0 flex h-dvh max-h-none w-full max-w-none translate-x-0 translate-y-0 flex-col gap-4 overflow-hidden rounded-none sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:min-h-[32rem] sm:max-h-[88vh] sm:max-w-4xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader>
          <DialogTitle>Launchers</DialogTitle>
          <DialogDescription>
            Configure reusable tmux window launchers for Codex, Claude Code, and
            any other command workflow.
          </DialogDescription>
        </DialogHeader>

        <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto md:overflow-hidden">
          <div className="grid gap-4 md:h-full md:grid-cols-[15rem_minmax(0,1fr)]">
            <section className="grid min-h-0 content-start gap-3 md:grid-rows-[auto_minmax(0,1fr)]">
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
                      const Icon = getTmuxIcon(preset.icon)
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
                    <ul className="grid min-h-0 content-start list-none gap-1 rounded-lg border border-border-subtle bg-secondary p-2 md:overflow-y-auto">
                      {launchers.map((launcher) => (
                        <SortableLauncherItem
                          key={launcher.id}
                          launcher={launcher}
                          selected={launcher.id === selectedID}
                          dragEnabled={dragEnabled}
                          onSelect={selectLauncher}
                        />
                      ))}
                    </ul>
                  </SortableContext>
                </DndContext>
              )}
            </section>

            <section className="grid min-h-0 gap-3 rounded-lg border border-border-subtle bg-secondary p-3 md:overflow-y-auto">
              <div className="grid gap-2 md:grid-cols-2">
                <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                  Name
                  <Input
                    className="bg-surface-overlay"
                    value={draft.name}
                    onChange={(event) =>
                      updateDraft((prev) => ({
                        ...prev,
                        name: event.target.value,
                      }))
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
                      {TMUX_ICONS.map((entry) => {
                        const Icon = entry.icon
                        return (
                          <DropdownMenuItem
                            key={entry.key}
                            className="cursor-pointer"
                            onSelect={() =>
                              updateDraft((prev) => ({
                                ...prev,
                                icon: entry.key,
                              }))
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
                    updateDraft((prev) => ({
                      ...prev,
                      command: event.target.value,
                    }))
                  }
                  placeholder="codex"
                />
                <span className="text-[11px] font-normal normal-case tracking-normal text-muted-foreground">
                  Leave blank to open a plain shell window.
                </span>
              </label>

              <div className="grid gap-2 md:grid-cols-2">
                <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.06em] text-secondary-foreground">
                  Working Directory
                  <Select
                    value={draft.cwdMode}
                    onValueChange={(value: LauncherCwdMode) =>
                      updateDraft((prev) => ({
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
                      <SelectItem
                        value="active-pane"
                        className="cursor-pointer"
                      >
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
                      updateDraft((prev) => ({
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
                      updateDraft((prev) => ({
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

              {saveError !== '' && (
                <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[12px] text-destructive">
                  {saveError}
                </div>
              )}

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
                    disabled={saving}
                  >
                    {saving ? 'Saving...' : 'Save'}
                  </Button>
                </div>
              </div>
            </section>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
