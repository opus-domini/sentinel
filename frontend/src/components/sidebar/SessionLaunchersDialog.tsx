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
import { useId, useMemo, useState } from 'react'
import type { SessionLauncher } from '@/types'
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
import { useViewport } from '@/contexts/ViewportContext'
import { hapticFeedback } from '@/lib/device'
import { DEFAULT_ICON_KEY } from '@/lib/tmuxIcons'
import { slugifyTmuxName } from '@/lib/tmuxName'

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
  const cwdLabelId = `${dialogId}-cwd-label`
  const userLabelId = `${dialogId}-user-label`
  const { touchOptimized: isMobile } = useViewport()
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
  const [dialogSource, setDialogSource] = useState({ open, launchers, defaultDraft })
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
    const launcher = launchers.find((item) => item.id === id)
    setDraft(launcher ? draftFromLauncher(launcher) : defaultDraft)
  }

  if (
    dialogSource.open !== open ||
    dialogSource.launchers !== launchers ||
    dialogSource.defaultDraft !== defaultDraft
  ) {
    setDialogSource({ open, launchers, defaultDraft })
    if (!open) {
      setSaveError('')
      setSaving(false)
      setSelectedID('new')
      setDraft(defaultDraft)
    } else if (selectedID === 'new') {
      setDraft(defaultDraft)
    } else {
      const selected = launchers.find((launcher) => launcher.id === selectedID)
      if (selected) {
        setDraft(draftFromLauncher(selected))
      } else {
        const next = launchers[0]
        setSelectedID(next?.id ?? 'new')
        setDraft(next ? draftFromLauncher(next) : defaultDraft)
      }
    }
  }

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
      const next = launchers.find((launcher) => launcher.id === nextID)
      setDraft(next ? draftFromLauncher(next) : defaultDraft)
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
                        <SortableLauncherRow
                          key={launcher.id}
                          id={launcher.id}
                          iconKey={launcher.icon}
                          name={launcher.name}
                          description={describeSessionLauncher(launcher)}
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

                <LauncherIconSelect
                  labelId={iconLabelId}
                  value={draft.icon}
                  onChange={(iconKey) =>
                    updateDraft((previous) => ({
                      ...previous,
                      icon: iconKey,
                    }))
                  }
                />
              </div>

              <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                <span id={cwdLabelId}>Working Directory</span>
                <DirectoryCombobox
                  id={cwdId}
                  ariaLabelledBy={cwdLabelId}
                  className="bg-surface-overlay font-mono"
                  value={draft.cwd}
                  open={open}
                  onChange={(next) =>
                    updateDraft((previous) => ({
                      ...previous,
                      cwd: next,
                    }))
                  }
                  placeholder={normalizedDefaultCwd || '/srv/app'}
                />
              </div>

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

              <LauncherDialogActions
                deleteTitle="Delete session launcher?"
                canDelete={draft.id !== ''}
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
