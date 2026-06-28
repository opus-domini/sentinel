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
import { useEffect, useId, useMemo, useState } from 'react'
import type { SessionLauncher } from '@/types'
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
  id: string
  name: string
  cwd: string
  icon: string
  user: string
}

type SessionLaunchersDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultCwd: string
  launchers: Array<SessionLauncher>
  onSave: (draft: SessionLauncherDraft) => Promise<string>
  onDelete: (id: string) => Promise<boolean>
  onReorder: (activeID: string, overID: string) => void
}

function createDefaultDraft(defaultCwd: string): SessionLauncherDraft {
  return {
    id: '',
    name: '',
    cwd: defaultCwd.trim(),
    icon: DEFAULT_ICON_KEY,
    user: '',
  }
}

function draftFromLauncher(launcher: SessionLauncher): SessionLauncherDraft {
  return {
    id: launcher.id,
    name: launcher.name,
    cwd: launcher.cwd,
    icon: launcher.icon,
    user: launcher.user ?? '',
  }
}

function describeSessionLauncher(launcher: Pick<SessionLauncher, 'cwd' | 'user'>) {
  const cwd = launcher.cwd.trim()
  const user = (launcher.user ?? '').trim()
  if (cwd === '') {
    return user === '' ? '' : user
  }
  if (user === '') {
    return cwd
  }
  return `${cwd} · ${user}`
}

function SortableSessionLauncherItem({
  launcher,
  selected,
  dragEnabled,
  onSelect,
}: {
  launcher: SessionLauncher
  selected: boolean
  dragEnabled: boolean
  onSelect: (id: string) => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: launcher.id,
  })
  const Icon = getTmuxIcon(launcher.icon)

  return (
    <li
      ref={setNodeRef}
      className="min-w-0 shrink-0"
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
          <span className="block truncate text-[12px] font-semibold">{launcher.name}</span>
          <span className="block truncate text-[10px] text-muted-foreground">
            {describeSessionLauncher(launcher)}
          </span>
        </span>
      </button>
    </li>
  )
}

export default function SessionLaunchersDialog({
  open,
  onOpenChange,
  defaultCwd,
  launchers,
  onSave,
  onDelete,
  onReorder,
}: SessionLaunchersDialogProps) {
  const meta = useMetaContext()
  const dialogId = useId()
  const nameId = `${dialogId}-name`
  const iconLabelId = `${dialogId}-icon-label`
  const cwdId = `${dialogId}-cwd`
  const userLabelId = `${dialogId}-user-label`
  const isMobile = useIsMobileLayout()
  const dragEnabled = !isMobile
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const defaultDraft = useMemo(
    () => createDefaultDraft(normalizedDefaultCwd),
    [normalizedDefaultCwd],
  )
  const [selectedID, setSelectedID] = useState<string>('new')
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
    setSelectedID('new')
    setDraft(defaultDraft)
  }

  const updateDraft = (updater: (previous: SessionLauncherDraft) => SessionLauncherDraft) => {
    setSaveError('')
    setDraft(updater)
  }

  const selectLauncher = (id: string) => {
    setSaveError('')
    setSelectedID(id)
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
      setDraft(defaultDraft)
      return
    }
    if (selectedID === 'new') {
      return
    }
    if (selectedLauncher === null) {
      setSelectedID(launchers[0]?.id ?? 'new')
    }
  }, [defaultDraft, open, launchers, selectedID, selectedLauncher])

  useEffect(() => {
    if (!open) {
      return
    }
    if (selectedLauncher !== null) {
      setDraft(draftFromLauncher(selectedLauncher))
      return
    }
    if (selectedID === 'new') {
      setDraft(defaultDraft)
    }
  }, [defaultDraft, open, selectedID, selectedLauncher])

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
      id: draft.id.trim(),
      name: normalizedName,
      cwd: normalizedCwd,
      user: draft.user.trim(),
    }

    try {
      const savedID = await onSave(nextDraft)
      if (savedID !== '') {
        setSelectedID(savedID)
        setDraft({
          ...nextDraft,
          id: savedID,
        })
        return
      }
      setSaveError('failed to save session launcher')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    const targetID = draft.id.trim()
    if (targetID === '') {
      return
    }

    const deleted = await onDelete(targetID)
    if (!deleted) {
      return
    }

    const nextID = launchers.find((launcher) => launcher.id !== targetID)?.id
    if (nextID) {
      setSelectedID(nextID)
      return
    }
    setSelectedID('new')
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
            Configure reusable tmux session launchers for common workspaces and users. These also
            appear in the `+` menu.
          </DialogDescription>
        </DialogHeader>

        <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto md:overflow-hidden">
          <div className="grid gap-4 md:h-full md:grid-cols-[15rem_minmax(0,1fr)]">
            <section className="grid min-h-0 min-w-0 content-start gap-3 md:grid-rows-[auto_minmax(0,1fr)]">
              <Button
                type="button"
                variant="outline"
                className="cursor-pointer justify-start"
                onClick={startNewLauncher}
              >
                New launcher
              </Button>

              {launchers.length === 0 ? (
                <EmptyState variant="inline" className="grid gap-2 p-3 text-left text-[12px]">
                  <span className="text-[12px]">No session launchers configured yet.</span>
                  <span className="text-muted-foreground">
                    Save a named session target here to make it available from the sidebar `+` menu.
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
                        <SortableSessionLauncherItem
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
                      updateDraft((previous) => ({
                        ...previous,
                        name: slugifyTmuxName(event.target.value),
                      }))
                    }
                    placeholder="api"
                  />
                </label>

                <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  <span id={iconLabelId}>Icon</span>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        aria-labelledby={iconLabelId}
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
                </div>
              </div>

              <label
                htmlFor={cwdId}
                className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground"
              >
                Working Directory
                <Input
                  id={cwdId}
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
                <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                  <span id={userLabelId}>Run as user</span>
                  <Select
                    value={draft.user === '' ? '__default__' : draft.user}
                    onValueChange={(value) =>
                      updateDraft((previous) => ({
                        ...previous,
                        user: value === '__default__' ? '' : value,
                      }))
                    }
                  >
                    <SelectTrigger
                      aria-labelledby={userLabelId}
                      className="w-full cursor-pointer bg-surface-overlay text-[12px]"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="z-[60]">
                      <SelectItem value="__default__" className="cursor-pointer">
                        Default user
                      </SelectItem>
                      {meta.allowedUsers.map((user) => (
                        <SelectItem key={user} value={user} className="cursor-pointer">
                          {user}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              <div className="rounded-md border border-border-subtle bg-surface-overlay px-3 py-2 text-[11px] text-muted-foreground">
                Session launchers stay available from the sidebar `+` menu until you delete them.
              </div>

              {saveError !== '' && (
                <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[12px] text-destructive">
                  {saveError}
                </div>
              )}

              <div className="mt-auto flex flex-wrap items-center gap-2">
                <div className="ml-auto flex items-center gap-2">
                  {draft.id !== '' && (
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
                          <AlertDialogTitle>Delete session launcher?</AlertDialogTitle>
                          <AlertDialogDescription>
                            This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction variant="destructive" onClick={handleDelete}>
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
