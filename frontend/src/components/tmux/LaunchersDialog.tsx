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
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { ChevronDown } from 'lucide-react'
import { useId, useState } from 'react'
import { TmuxIcon } from '@/lib/tmuxIcons'
import type { LauncherCwdMode, LauncherUserMode, TmuxLauncher } from '@/types'
import DirectoryCombobox from '@/components/DirectoryCombobox'
import LauncherDialogActions from '@/components/launcher/LauncherDialogActions'
import LauncherIconSelect from '@/components/launcher/LauncherIconSelect'
import SortableLauncherRow from '@/components/launcher/SortableLauncherRow'
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
import { useViewport } from '@/contexts/ViewportContext'
import { hapticFeedback } from '@/lib/device'
import { useMetaContext } from '@/contexts/MetaContext'

export type LauncherDraft = {
  id?: string
  name: string
  icon: string
  command: string
  cwdMode: LauncherCwdMode
  cwdValue: string
  windowName: string
  userMode: LauncherUserMode
  userValue: string
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
  userMode: 'session',
  userValue: '',
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
    userMode: launcher.userMode,
    userValue: launcher.userValue ?? '',
  }
}

function describeLauncherCommand(command: string) {
  const normalized = command.trim()
  if (normalized !== '') {
    return normalized
  }
  return 'plain shell'
}

export default function LaunchersDialog({
  open,
  onOpenChange,
  launchers,
  onSave,
  onDelete,
  onReorder,
}: LaunchersDialogProps) {
  const meta = useMetaContext()
  const dialogId = useId()
  const nameId = `${dialogId}-name`
  const iconLabelId = `${dialogId}-icon-label`
  const commandId = `${dialogId}-command`
  const cwdModeLabelId = `${dialogId}-cwd-mode-label`
  const windowNameId = `${dialogId}-window-name`
  const fixedPathId = `${dialogId}-fixed-path`
  const fixedPathLabelId = `${dialogId}-fixed-path-label`
  const userModeLabelId = `${dialogId}-user-mode-label`
  const fixedUserLabelId = `${dialogId}-fixed-user-label`
  const { touchOptimized: isMobile } = useViewport()
  const dragEnabled = !isMobile
  const [selectedID, setSelectedID] = useState<string>('new')
  const [draft, setDraft] = useState<LauncherDraft>(DEFAULT_DRAFT)
  const [saveError, setSaveError] = useState('')
  const [saving, setSaving] = useState(false)
  const [dialogSource, setDialogSource] = useState({ open, launchers })
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
    const launcher = launchers.find((item) => item.id === id)
    setDraft(launcher ? draftFromLauncher(launcher) : DEFAULT_DRAFT)
  }

  const applyQuickStart = (preset: (typeof QUICK_STARTS)[number]) => {
    setSaveError('')
    setSelectedID('new')
    setDraft({ ...DEFAULT_DRAFT, ...preset })
  }

  if (dialogSource.open !== open || dialogSource.launchers !== launchers) {
    setDialogSource({ open, launchers })
    if (!open) {
      setSaveError('')
      setSaving(false)
      setSelectedID('new')
      setDraft(DEFAULT_DRAFT)
    } else if (selectedID === 'new') {
      setDraft(DEFAULT_DRAFT)
    } else {
      const selected = launchers.find((launcher) => launcher.id === selectedID)
      if (selected) {
        setDraft(draftFromLauncher(selected))
      } else {
        const next = launchers[0]
        setSelectedID(next?.id ?? 'new')
        setDraft(next ? draftFromLauncher(next) : DEFAULT_DRAFT)
      }
    }
  }

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
      const next = launchers.find((item) => item.id !== draft.id)
      setSelectedID(next?.id ?? 'new')
      setDraft(next ? draftFromLauncher(next) : DEFAULT_DRAFT)
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
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="inset-0 flex h-dvh max-h-none w-full max-w-none translate-x-0 translate-y-0 flex-col gap-4 overflow-hidden rounded-none sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:min-h-[32rem] sm:max-h-[88vh] sm:max-w-4xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader>
          <DialogTitle>Launchers</DialogTitle>
          <DialogDescription>
            Configure reusable tmux window launchers for Codex, Claude Code, and any other command
            workflow.
          </DialogDescription>
        </DialogHeader>

        <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto md:overflow-hidden">
          <div className="grid gap-4 md:h-full md:grid-cols-[15rem_minmax(0,1fr)]">
            <section className="grid min-h-0 min-w-0 content-start gap-3 md:grid-rows-[auto_minmax(0,1fr)]">
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
                    <DropdownMenuItem className="cursor-pointer" onSelect={startNewLauncher}>
                      Blank launcher
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuLabel>Starter presets</DropdownMenuLabel>
                    {QUICK_STARTS.map((preset) => {
                      return (
                        <DropdownMenuItem
                          key={preset.name}
                          className="cursor-pointer"
                          onSelect={() => applyQuickStart(preset)}
                        >
                          <TmuxIcon iconKey={preset.icon} className="h-3.5 w-3.5" />
                          {preset.name}
                        </DropdownMenuItem>
                      )
                    })}
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              {launchers.length === 0 ? (
                <EmptyState variant="inline" className="grid gap-2 p-3 text-left text-[12px]">
                  <span className="text-[12px]">No launchers configured yet.</span>
                  <span className="text-muted-foreground">
                    Start from a blank launcher or pick a preset from the split button above.
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
                    <ul className="flex min-h-0 min-w-0 flex-col list-none gap-1 overflow-x-hidden rounded-lg border border-border-subtle bg-secondary p-2 md:overflow-y-auto">
                      {launchers.map((launcher) => (
                        <SortableLauncherRow
                          key={launcher.id}
                          id={launcher.id}
                          iconKey={launcher.icon}
                          name={launcher.name}
                          description={describeLauncherCommand(launcher.command)}
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

            <section className="grid min-h-0 content-start gap-3 rounded-lg border border-border-subtle bg-secondary p-3 md:overflow-y-auto">
              <div className="grid gap-2 md:grid-cols-2">
                <label
                  htmlFor={nameId}
                  className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground"
                >
                  Name
                  <Input
                    id={nameId}
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

                <LauncherIconSelect
                  labelId={iconLabelId}
                  value={draft.icon}
                  onChange={(iconKey) =>
                    updateDraft((prev) => ({
                      ...prev,
                      icon: iconKey,
                    }))
                  }
                />
              </div>

              <label
                htmlFor={commandId}
                className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground"
              >
                Command
                <Input
                  id={commandId}
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
                <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  <span id={cwdModeLabelId}>Working Directory</span>
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
                    <SelectTrigger
                      aria-labelledby={cwdModeLabelId}
                      className="w-full cursor-pointer bg-surface-overlay text-[12px]"
                    >
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
                </div>

                <label
                  htmlFor={windowNameId}
                  className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground"
                >
                  Window Name
                  <Input
                    id={windowNameId}
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
                <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  <span id={fixedPathLabelId}>Fixed Path</span>
                  <DirectoryCombobox
                    id={fixedPathId}
                    ariaLabelledBy={fixedPathLabelId}
                    className="bg-surface-overlay font-mono"
                    value={draft.cwdValue}
                    open={open}
                    onChange={(next) =>
                      updateDraft((prev) => ({
                        ...prev,
                        cwdValue: next,
                      }))
                    }
                    placeholder="/home/hugo/project"
                  />
                </div>
              )}

              {meta.canSwitchUser && (
                <>
                  <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                    <span id={userModeLabelId}>Run as user</span>
                    <Select
                      value={draft.userMode}
                      onValueChange={(value: LauncherUserMode) =>
                        updateDraft((prev) => ({
                          ...prev,
                          userMode: value,
                          userValue: value === 'fixed' ? prev.userValue : '',
                        }))
                      }
                    >
                      <SelectTrigger
                        aria-labelledby={userModeLabelId}
                        className="w-full cursor-pointer bg-surface-overlay text-[12px]"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent className="z-[60]">
                        <SelectItem value="session" className="cursor-pointer">
                          session user
                        </SelectItem>
                        <SelectItem value="fixed" className="cursor-pointer">
                          fixed user
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <span className="text-[11px] font-normal normal-case tracking-normal text-muted-foreground">
                      Override the system user for this window. Leave blank to inherit from the
                      session.
                    </span>
                  </div>
                  {draft.userMode === 'fixed' && (
                    <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                      <span id={fixedUserLabelId}>Fixed User</span>
                      <Select
                        value={draft.userValue}
                        onValueChange={(value) =>
                          updateDraft((prev) => ({
                            ...prev,
                            userValue: value,
                          }))
                        }
                      >
                        <SelectTrigger
                          aria-labelledby={fixedUserLabelId}
                          className="w-full cursor-pointer bg-surface-overlay text-[12px]"
                        >
                          <SelectValue placeholder="Select user" />
                        </SelectTrigger>
                        <SelectContent className="z-[60]">
                          {meta.allowedUsers.map((u) => (
                            <SelectItem key={u} value={u} className="cursor-pointer">
                              {u}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                </>
              )}

              <div className="rounded-md border border-border-subtle bg-surface-overlay px-3 py-2 text-[11px] text-muted-foreground">
                Launchers always open a new tmux window from the active session. The `+` menu
                becomes the fast path to use them.
              </div>

              {saveError !== '' && (
                <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[12px] text-destructive">
                  {saveError}
                </div>
              )}

              <LauncherDialogActions
                deleteTitle="Delete launcher?"
                canDelete={Boolean(draft.id)}
                saving={saving}
                onDelete={handleDelete}
                onSave={handleSave}
              />
            </section>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
