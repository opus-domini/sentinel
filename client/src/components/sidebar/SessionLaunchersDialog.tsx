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
import { Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { SessionPreset } from '@/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
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
import { useMetaContext } from '@/contexts/MetaContext'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { hapticFeedback } from '@/lib/device'
import { DEFAULT_ICON_KEY, TMUX_ICONS, getTmuxIcon } from '@/lib/tmuxIcons'
import { slugifyTmuxName } from '@/lib/tmuxName'
import { cn } from '@/lib/utils'

export type SessionLauncherDraft = {
  previousName: string
  name: string
  cwd: string
  icon: string
  user: string
}

type SessionLaunchersDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultCwd: string
  presets: Array<SessionPreset>
  onSave: (draft: SessionLauncherDraft) => Promise<boolean>
  onDelete: (name: string) => Promise<boolean>
  onReorder: (activeName: string, overName: string) => void
}

function createDefaultDraft(defaultCwd: string): SessionLauncherDraft {
  return {
    previousName: '',
    name: '',
    cwd: defaultCwd.trim(),
    icon: DEFAULT_ICON_KEY,
    user: '',
  }
}

function draftFromPreset(preset: SessionPreset): SessionLauncherDraft {
  return {
    previousName: preset.name,
    name: preset.name,
    cwd: preset.cwd,
    icon: preset.icon,
    user: preset.user ?? '',
  }
}

function describeSessionLauncher(preset: Pick<SessionPreset, 'cwd' | 'user'>) {
  const cwd = preset.cwd.trim()
  const user = (preset.user ?? '').trim()
  if (cwd === '') {
    return user === '' ? '' : user
  }
  if (user === '') {
    return cwd
  }
  return `${cwd} · ${user}`
}

function SortableSessionLauncherItem({
  preset,
  selected,
  dragEnabled,
  onSelect,
}: {
  preset: SessionPreset
  selected: boolean
  dragEnabled: boolean
  onSelect: (name: string) => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: preset.name })
  const Icon = getTmuxIcon(preset.icon)

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
        onClick={() => onSelect(preset.name)}
        style={{ touchAction: dragEnabled ? undefined : 'pan-y' }}
        {...(dragEnabled ? attributes : {})}
        {...(dragEnabled ? listeners : {})}
      >
        <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12px] font-semibold">
            {preset.name}
          </span>
          <span className="block truncate text-[10px] text-muted-foreground">
            {describeSessionLauncher(preset)}
          </span>
        </span>
        {!preset.lastLaunchedAt && (
          <span className="text-[10px] text-muted-foreground">New</span>
        )}
      </button>
    </li>
  )
}

export default function SessionLaunchersDialog({
  open,
  onOpenChange,
  defaultCwd,
  presets,
  onSave,
  onDelete,
  onReorder,
}: SessionLaunchersDialogProps) {
  const meta = useMetaContext()
  const isMobile = useIsMobileLayout()
  const dragEnabled = !isMobile
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const defaultDraft = useMemo(
    () => createDefaultDraft(normalizedDefaultCwd),
    [normalizedDefaultCwd],
  )
  const [selectedName, setSelectedName] = useState<string>('new')
  const [draft, setDraft] = useState<SessionLauncherDraft>(defaultDraft)
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
    setSelectedName('new')
    setDraft(defaultDraft)
  }

  const updateDraft = (
    updater: (previous: SessionLauncherDraft) => SessionLauncherDraft,
  ) => {
    setSaveError('')
    setDraft(updater)
  }

  const selectLauncher = (name: string) => {
    setSaveError('')
    setSelectedName(name)
  }

  const selectedPreset = useMemo(
    () => presets.find((preset) => preset.name === selectedName) ?? null,
    [presets, selectedName],
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
      setSelectedName('new')
      setDraft(defaultDraft)
      return
    }
    if (selectedName === 'new') {
      return
    }
    if (selectedPreset === null) {
      setSelectedName(presets[0]?.name ?? 'new')
    }
  }, [defaultDraft, open, presets, selectedName, selectedPreset])

  useEffect(() => {
    if (!open) {
      return
    }
    if (selectedPreset !== null) {
      setDraft(draftFromPreset(selectedPreset))
      return
    }
    if (selectedName === 'new') {
      setDraft(defaultDraft)
    }
  }, [defaultDraft, open, selectedName, selectedPreset])

  const handleSave = async () => {
    const normalizedName = slugifyTmuxName(draft.name).trim()
    const normalizedCwd = draft.cwd.trim()
    if (normalizedName === '') {
      setSaveError('session launcher name is required')
      return
    }
    if (normalizedCwd === '') {
      setSaveError('working directory is required')
      return
    }

    setSaving(true)
    setSaveError('')
    const nextDraft = {
      ...draft,
      previousName: draft.previousName.trim(),
      name: normalizedName,
      cwd: normalizedCwd,
      user: draft.user.trim(),
    }

    try {
      const saved = await onSave(nextDraft)
      if (saved) {
        setSelectedName(nextDraft.name)
        setDraft({
          ...nextDraft,
          previousName: nextDraft.name,
        })
        return
      }
      setSaveError('failed to save session launcher')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    const targetName = draft.previousName.trim()
    if (targetName === '') {
      return
    }

    const deleted = await onDelete(targetName)
    if (!deleted) {
      return
    }

    const nextName = presets.find((preset) => preset.name !== targetName)?.name
    if (nextName) {
      setSelectedName(nextName)
      return
    }
    setSelectedName('new')
    setDraft(defaultDraft)
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
      <DialogContent className="inset-0 flex h-dvh max-h-none w-full max-w-none translate-x-0 translate-y-0 flex-col gap-4 overflow-hidden rounded-none sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:min-h-[30rem] sm:max-h-[88vh] sm:max-w-4xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader>
          <DialogTitle>Session Launchers</DialogTitle>
          <DialogDescription>
            Configure reusable tmux session launchers for common workspaces and
            users. These also appear in the `+` menu.
          </DialogDescription>
        </DialogHeader>

        <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto md:overflow-hidden">
          <div className="grid gap-4 md:h-full md:grid-cols-[15rem_minmax(0,1fr)]">
            <section className="grid min-h-0 content-start gap-3 md:grid-rows-[auto_minmax(0,1fr)]">
              <Button
                type="button"
                variant="outline"
                className="cursor-pointer justify-start"
                onClick={startNewLauncher}
              >
                New launcher
              </Button>

              {presets.length === 0 ? (
                <EmptyState
                  variant="inline"
                  className="grid gap-2 p-3 text-left text-[12px]"
                >
                  <span className="text-[12px]">
                    No session launchers configured yet.
                  </span>
                  <span className="text-muted-foreground">
                    Save a named session target here to make it available from
                    the sidebar `+` menu.
                  </span>
                </EmptyState>
              ) : (
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragEnd={handleDragEnd}
                >
                  <SortableContext
                    items={presets.map((preset) => preset.name)}
                    strategy={verticalListSortingStrategy}
                  >
                    <ul className="grid min-h-0 content-start list-none gap-1 rounded-lg border border-border-subtle bg-secondary p-2 md:overflow-y-auto">
                      {presets.map((preset) => (
                        <SortableSessionLauncherItem
                          key={preset.name}
                          preset={preset}
                          selected={preset.name === selectedName}
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
                <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  Name
                  <Input
                    className="bg-surface-overlay"
                    value={draft.name}
                    onChange={(event) =>
                      updateDraft((previous) => ({
                        ...previous,
                        name: slugifyTmuxName(event.target.value),
                      }))
                    }
                    placeholder="api"
                  />
                </label>

                <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
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
                              updateDraft((previous) => ({
                                ...previous,
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

              <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                Working Directory
                <Input
                  className="bg-surface-overlay font-mono"
                  value={draft.cwd}
                  onChange={(event) =>
                    updateDraft((previous) => ({
                      ...previous,
                      cwd: event.target.value,
                    }))
                  }
                  placeholder={normalizedDefaultCwd || '/srv/app'}
                />
              </label>

              {meta.canSwitchUser && (
                <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  Run as user
                  <Select
                    value={draft.user === '' ? '__default__' : draft.user}
                    onValueChange={(value) =>
                      updateDraft((previous) => ({
                        ...previous,
                        user: value === '__default__' ? '' : value,
                      }))
                    }
                  >
                    <SelectTrigger className="w-full cursor-pointer bg-surface-overlay text-[12px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="z-[60]">
                      <SelectItem
                        value="__default__"
                        className="cursor-pointer"
                      >
                        Default user
                      </SelectItem>
                      {meta.allowedUsers.map((user) => (
                        <SelectItem
                          key={user}
                          value={user}
                          className="cursor-pointer"
                        >
                          {user}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </label>
              )}

              <div className="rounded-md border border-border-subtle bg-surface-overlay px-3 py-2 text-[11px] text-muted-foreground">
                Session launchers show up in the sidebar `+` menu and also stay
                available under Pinned while their session is offline.
              </div>

              {saveError !== '' && (
                <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[12px] text-destructive">
                  {saveError}
                </div>
              )}

              <div className="mt-auto flex flex-wrap items-center gap-2">
                <div className="ml-auto flex items-center gap-2">
                  {draft.previousName !== '' && (
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <Button
                          type="button"
                          variant="destructive"
                          size="sm"
                          className="cursor-pointer"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                          Delete
                        </Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>
                            Delete session launcher?
                          </AlertDialogTitle>
                          <AlertDialogDescription>
                            This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction
                            variant="destructive"
                            onClick={handleDelete}
                          >
                            Delete
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
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
