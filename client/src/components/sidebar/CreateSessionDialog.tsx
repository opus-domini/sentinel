import { useEffect, useMemo, useState } from 'react'
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
import { slugifyTmuxName } from '@/lib/tmuxName'

type CreateSessionDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultCwd: string
  token: string
  onCreate: (name: string, cwd: string) => void
}

type DirectorySuggestionsResponse = {
  dirs?: Array<string>
}

export default function CreateSessionDialog({
  open,
  onOpenChange,
  defaultCwd,
  token,
  onCreate,
}: CreateSessionDialogProps) {
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState(normalizedDefaultCwd)
  const [cwdSuggestions, setCwdSuggestions] = useState<Array<string>>([])
  const [cwdLoading, setCwdLoading] = useState(false)
  const [cwdFocused, setCwdFocused] = useState(false)
  const [activeSuggestion, setActiveSuggestion] = useState(-1)

  useEffect(() => {
    if (!open) {
      setCwd(normalizedDefaultCwd)
      setCwdSuggestions([])
      setCwdLoading(false)
      setCwdFocused(false)
      setActiveSuggestion(-1)
    }
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
          const headers: Record<string, string> = {
            Accept: 'application/json',
          }
          if (token.trim() !== '') {
            headers.Authorization = `Bearer ${token.trim()}`
          }

          const response = await fetch(
            `/api/fs/dirs?prefix=${encodeURIComponent(query)}&limit=10`,
            {
              signal: abort.signal,
              headers,
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
  }, [cwd, normalizedDefaultCwd, open, token])

  function selectSuggestion(value: string) {
    setCwd(value)
    setCwdSuggestions([])
    setActiveSuggestion(-1)
    setCwdFocused(true)
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setName('')
      setCwd(normalizedDefaultCwd)
      setCwdSuggestions([])
      setCwdLoading(false)
      setCwdFocused(false)
      setActiveSuggestion(-1)
    }
    onOpenChange(next)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return
    onCreate(trimmed, cwd.trim())
    setName('')
    setCwd(normalizedDefaultCwd)
    setCwdSuggestions([])
    setCwdLoading(false)
    setCwdFocused(false)
    setActiveSuggestion(-1)
    onOpenChange(false)
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
                  <div className="absolute left-0 right-0 z-20 mt-1 max-h-44 overflow-auto rounded-md border border-border bg-popover p-1 shadow-md">
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
          </div>
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>
            <Button type="submit" disabled={!name.trim()}>
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
