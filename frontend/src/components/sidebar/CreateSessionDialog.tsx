import { ChevronRight } from 'lucide-react'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import DirectoryCombobox from '@/components/DirectoryCombobox'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useMetaContext } from '@/contexts/MetaContext'
import { slugifyTmuxName } from '@/lib/tmuxName'

type CreateSessionDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultCwd: string
  onCreate: (name: string, cwd: string, user?: string) => Promise<void>
}

type DirectorySuggestionsResponse = {
  dirs?: Array<string>
}

export default function CreateSessionDialog({
  open,
  onOpenChange,
  defaultCwd,
  onCreate,
}: CreateSessionDialogProps) {
  const meta = useMetaContext()
  const id = useId()
  const nameId = `${id}-name`
  const cwdId = `${id}-cwd`
  const errorId = `${id}-error`
  const runAsUserLabelId = `${id}-run-as-user-label`
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState(normalizedDefaultCwd)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [frequentDirs, setFrequentDirs] = useState<Array<string>>([])
  const frequentDirsFetched = useRef(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [user, setUser] = useState('')

  const resetForm = useCallback(() => {
    setName('')
    setCwd(normalizedDefaultCwd)
    setSaving(false)
    setError('')
    setAdvancedOpen(false)
    setUser('')
  }, [normalizedDefaultCwd])

  useEffect(() => {
    if (!open) {
      resetForm()
      frequentDirsFetched.current = false
      return
    }

    if (frequentDirsFetched.current) return
    frequentDirsFetched.current = true

    const abort = new AbortController()
    void (async () => {
      try {
        const response = await fetch('/api/tmux/frequent-dirs?limit=5', {
          signal: abort.signal,
          headers: { Accept: 'application/json' },
          credentials: 'same-origin',
        })
        if (!response.ok) return
        const payload = (await response.json()) as {
          data?: DirectorySuggestionsResponse
        }
        const rawDirs = payload.data?.dirs
        const dirs = Array.isArray(rawDirs)
          ? rawDirs.filter((item): item is string => typeof item === 'string')
          : []
        setFrequentDirs(dirs)
      } catch {
        // ignore
      }
    })()

    return () => abort.abort()
  }, [normalizedDefaultCwd, open, resetForm])

  const filteredFrequentDirs = useMemo(() => {
    if (!normalizedDefaultCwd) return frequentDirs
    return frequentDirs.filter((d) => d !== normalizedDefaultCwd)
  }, [frequentDirs, normalizedDefaultCwd])

  function shortenPath(path: string): string {
    if (normalizedDefaultCwd && path.startsWith(`${normalizedDefaultCwd}/`)) {
      return `~/${path.slice(normalizedDefaultCwd.length + 1)}`
    }
    return path
  }

  function selectFrequentDir(path: string) {
    setCwd(path)
    setError('')
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      if (saving) return
      resetForm()
    }
    onOpenChange(next)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed || saving) return
    setSaving(true)
    setError('')
    try {
      const trimmedUser = user.trim()
      await onCreate(trimmed, cwd.trim(), trimmedUser || undefined)
      resetForm()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create session')
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New session</DialogTitle>
          <DialogDescription>Create a new tmux session.</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="grid gap-2">
            <Input
              id={nameId}
              aria-label="Session name"
              placeholder="session name"
              value={name}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              onChange={(e) => {
                setName(slugifyTmuxName(e.target.value))
                setError('')
              }}
            />
            <DirectoryCombobox
              id={cwdId}
              ariaLabel="Working directory"
              placeholder="working directory"
              value={cwd}
              open={open}
              fallbackPrefix={normalizedDefaultCwd}
              onChange={(next) => {
                setCwd(next)
                setError('')
              }}
            />
            {filteredFrequentDirs.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {filteredFrequentDirs.map((dir) => (
                  <button
                    key={dir}
                    type="button"
                    className="cursor-pointer rounded-full border border-border-subtle bg-secondary px-2.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                    onClick={() => selectFrequentDir(dir)}
                  >
                    {shortenPath(dir)}
                  </button>
                ))}
              </div>
            )}

            {error !== '' && (
              <p id={errorId} role="alert" className="text-xs text-destructive-foreground">
                {error}
              </p>
            )}

            {meta.canSwitchUser && (
              <div>
                <button
                  type="button"
                  className="flex cursor-pointer items-center gap-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
                  onClick={() => setAdvancedOpen((prev) => !prev)}
                >
                  <ChevronRight
                    className="h-3 w-3 transition-transform"
                    style={{
                      transform: advancedOpen ? 'rotate(90deg)' : undefined,
                    }}
                  />
                  Advanced options
                </button>
                {advancedOpen && (
                  <div className="mt-2 rounded-md border border-border-subtle p-2.5">
                    <div className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                      <span id={runAsUserLabelId}>Run as user</span>
                      <Select
                        value={user || '__default__'}
                        onValueChange={(v) => setUser(v === '__default__' ? '' : v)}
                      >
                        <SelectTrigger
                          aria-labelledby={runAsUserLabelId}
                          className="w-full cursor-pointer bg-surface-overlay text-[12px]"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__default__" className="cursor-pointer">
                            {meta.processUser || 'default'} (default)
                          </SelectItem>
                          {meta.allowedUsers
                            .filter((u) => u !== meta.processUser)
                            .map((u) => (
                              <SelectItem key={u} value={u} className="cursor-pointer">
                                {u}
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline" disabled={saving}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={!name.trim() || saving}>
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
