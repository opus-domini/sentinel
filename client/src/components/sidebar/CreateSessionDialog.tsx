// Uses a custom autocomplete instead of the shared Combobox (ui/combobox.tsx)
// because the working-directory field requires free-form text input with
// server-side filesystem path suggestions fetched on each keystroke. The shared
// Combobox wraps Base UI's Combobox primitive, which restricts selection to
// predefined items and does not support free-form values. Base UI's Autocomplete
// would be the correct primitive, but it is not yet available in the project.
import { ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
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
  onCreate: (name: string, cwd: string, user?: string) => void | Promise<void>
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
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState(normalizedDefaultCwd)
  const [cwdSuggestions, setCwdSuggestions] = useState<Array<string>>([])
  const [cwdLoading, setCwdLoading] = useState(false)
  const [cwdFocused, setCwdFocused] = useState(false)
  const [activeSuggestion, setActiveSuggestion] = useState(-1)
  const [saving, setSaving] = useState(false)
  const [frequentDirs, setFrequentDirs] = useState<Array<string>>([])
  const frequentDirsFetched = useRef(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [user, setUser] = useState('')

  function resetForm() {
    setName('')
    setCwd(normalizedDefaultCwd)
    setCwdSuggestions([])
    setCwdLoading(false)
    setCwdFocused(false)
    setActiveSuggestion(-1)
    setSaving(false)
    setAdvancedOpen(false)
    setUser('')
  }

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
  }, [normalizedDefaultCwd, open])

  useEffect(() => {
    if (!open) return

    const query = cwd.trim() || normalizedDefaultCwd
    if (query === '') {
      setCwdSuggestions([])
      setCwdLoading(false)
      setActiveSuggestion(-1)
      return
    }

    const abort = new AbortController()
    const timer = window.setTimeout(() => {
      void (async () => {
        setCwdLoading(true)
        try {
          const response = await fetch(
            `/api/fs/dirs?prefix=${encodeURIComponent(query)}&limit=10`,
            {
              signal: abort.signal,
              headers: { Accept: 'application/json' },
              credentials: 'same-origin',
            },
          )
          if (!response.ok) {
            setCwdSuggestions([])
            return
          }
          const payload = (await response.json()) as {
            data?: DirectorySuggestionsResponse
          }
          const rawDirs = payload.data?.dirs
          const dirs = Array.isArray(rawDirs)
            ? rawDirs.filter((item): item is string => typeof item === 'string')
            : []
          setCwdSuggestions(dirs)
          setActiveSuggestion(-1)
        } catch {
          if (!abort.signal.aborted) {
            setCwdSuggestions([])
            setActiveSuggestion(-1)
          }
        } finally {
          if (!abort.signal.aborted) {
            setCwdLoading(false)
          }
        }
      })()
    }, 140)

    return () => {
      window.clearTimeout(timer)
      abort.abort()
    }
  }, [cwd, normalizedDefaultCwd, open])

  const filteredFrequentDirs = useMemo(() => {
    if (!normalizedDefaultCwd) return frequentDirs
    return frequentDirs.filter((d) => d !== normalizedDefaultCwd)
  }, [frequentDirs, normalizedDefaultCwd])

  function shortenPath(path: string): string {
    if (normalizedDefaultCwd && path.startsWith(normalizedDefaultCwd + '/')) {
      return '~/' + path.slice(normalizedDefaultCwd.length + 1)
    }
    return path
  }

  function selectFrequentDir(path: string) {
    setCwd(path)
    setCwdSuggestions([])
    setActiveSuggestion(-1)
  }

  function selectSuggestion(value: string) {
    setCwd(value)
    setCwdSuggestions([])
    setActiveSuggestion(-1)
    setCwdFocused(true)
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      resetForm()
    }
    onOpenChange(next)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed || saving) return
    setSaving(true)
    try {
      const trimmedUser = user.trim()
      await onCreate(trimmed, cwd.trim(), trimmedUser || undefined)
    } finally {
      resetForm()
      onOpenChange(false)
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
              placeholder="session name"
              value={name}
              onChange={(e) => setName(slugifyTmuxName(e.target.value))}
              autoFocus
            />
            <div className="relative">
              <Input
                placeholder="working directory"
                value={cwd}
                role="combobox"
                aria-expanded={
                  cwdFocused && (cwdLoading || cwdSuggestions.length > 0)
                }
                aria-autocomplete="list"
                aria-controls="cwd-listbox"
                onChange={(e) => {
                  setCwd(e.target.value)
                  setActiveSuggestion(-1)
                }}
                onFocus={() => setCwdFocused(true)}
                onBlur={() => {
                  window.setTimeout(() => {
                    setCwdFocused(false)
                    setActiveSuggestion(-1)
                  }, 80)
                }}
                onKeyDown={(event) => {
                  if (event.key === 'Escape') {
                    setCwdSuggestions([])
                    setActiveSuggestion(-1)
                    return
                  }
                  if (cwdSuggestions.length === 0) return

                  if (event.key === 'ArrowDown') {
                    event.preventDefault()
                    setActiveSuggestion((prev) =>
                      Math.min(prev + 1, cwdSuggestions.length - 1),
                    )
                    return
                  }
                  if (event.key === 'ArrowUp') {
                    event.preventDefault()
                    setActiveSuggestion((prev) => Math.max(prev - 1, 0))
                    return
                  }
                  if (event.key === 'Enter' || event.key === 'Tab') {
                    const idx = activeSuggestion
                    if (idx >= 0 && idx < cwdSuggestions.length) {
                      event.preventDefault()
                      selectSuggestion(cwdSuggestions[idx])
                      return
                    }
                    if (event.key === 'Tab' && cwdSuggestions.length === 1) {
                      event.preventDefault()
                      selectSuggestion(cwdSuggestions[0])
                    }
                  }
                }}
              />

              {open &&
                cwdFocused &&
                (cwdLoading || cwdSuggestions.length > 0) && (
                  <div
                    id="cwd-listbox"
                    role="listbox"
                    className="absolute left-0 right-0 z-20 mt-1 max-h-44 overflow-auto rounded-md border border-border bg-popover p-1 shadow-md"
                  >
                    {cwdLoading && (
                      <div className="px-2 py-1 text-[11px] text-secondary-foreground">
                        Searching directories...
                      </div>
                    )}
                    {!cwdLoading &&
                      cwdSuggestions.map((item, idx) => (
                        <button
                          key={item}
                          type="button"
                          role="option"
                          aria-selected={idx === activeSuggestion}
                          className={`block w-full truncate rounded px-2 py-1 text-left text-[11px] ${
                            idx === activeSuggestion
                              ? 'bg-accent text-accent-foreground'
                              : 'hover:bg-secondary'
                          }`}
                          onMouseDown={(event) => {
                            event.preventDefault()
                            selectSuggestion(item)
                          }}
                        >
                          {item}
                        </button>
                      ))}
                  </div>
                )}
            </div>
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
                    <label className="grid gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-secondary-foreground">
                      Run as user
                      <Select value={user} onValueChange={setUser}>
                        <SelectTrigger className="w-full cursor-pointer bg-surface-overlay text-[12px]">
                          <SelectValue
                            placeholder={`${meta.processUser || 'default'} (default)`}
                          />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="" className="cursor-pointer">
                            {meta.processUser || 'default'} (default)
                          </SelectItem>
                          {meta.allowedUsers
                            .filter((u) => u !== meta.processUser)
                            .map((u) => (
                              <SelectItem
                                key={u}
                                value={u}
                                className="cursor-pointer"
                              >
                                {u}
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                    </label>
                  </div>
                )}
              </div>
            )}
          </div>
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
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
