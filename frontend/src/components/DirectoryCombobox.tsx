// Custom combobox for free-form working-directory input with server-side
// filesystem path suggestions fetched (debounced) on each keystroke.
import { useEffect, useId, useRef, useState } from 'react'
import { Input } from '@/components/ui/input'

type DirectorySuggestionsResponse = {
  dirs?: Array<string>
}

type DirectoryComboboxProps = {
  value: string
  onChange: (value: string) => void
  id?: string
  ariaLabel?: string
  ariaLabelledBy?: string
  placeholder?: string
  className?: string
  // When rendered inside a dialog, pass the dialog's open state so the suggestion
  // listbox is suppressed the instant the dialog starts closing (it stays mounted
  // during the close animation).
  open?: boolean
  // Prefix queried when the field is empty (e.g. the session's default cwd), so
  // suggestions are still offered before the user types anything.
  fallbackPrefix?: string
  limit?: number
}

export default function DirectoryCombobox({
  value,
  onChange,
  id,
  ariaLabel,
  ariaLabelledBy,
  placeholder,
  className,
  open = true,
  fallbackPrefix = '',
  limit = 10,
}: DirectoryComboboxProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const blurTimerRef = useRef<number | undefined>(undefined)
  const listboxId = `${useId()}-listbox`
  const [suggestions, setSuggestions] = useState<Array<string>>([])
  const [loading, setLoading] = useState(false)
  const [focused, setFocused] = useState(false)
  const [activeSuggestion, setActiveSuggestion] = useState(-1)
  // Set when a pointer press lands outside the field; keeps the listbox from
  // reopening (via a value-driven refetch) until the user types again.
  const [suppressed, setSuppressed] = useState(false)

  useEffect(() => {
    if (!open) return
    const query = value.trim() || fallbackPrefix.trim()
    if (query === '') {
      setSuggestions([])
      setLoading(false)
      setActiveSuggestion(-1)
      return
    }

    const abort = new AbortController()
    const timer = window.setTimeout(() => {
      void (async () => {
        setLoading(true)
        try {
          const response = await fetch(
            `/api/fs/dirs?prefix=${encodeURIComponent(query)}&limit=${limit}`,
            {
              signal: abort.signal,
              headers: { Accept: 'application/json' },
              credentials: 'same-origin',
            },
          )
          if (!response.ok) {
            setSuggestions([])
            return
          }
          const payload = (await response.json()) as {
            data?: DirectorySuggestionsResponse
          }
          const rawDirs = payload.data?.dirs
          const dirs = Array.isArray(rawDirs)
            ? rawDirs.filter((item): item is string => typeof item === 'string')
            : []
          setSuggestions(dirs)
          setActiveSuggestion(-1)
        } catch {
          if (!abort.signal.aborted) {
            setSuggestions([])
            setActiveSuggestion(-1)
          }
        } finally {
          if (!abort.signal.aborted) {
            setLoading(false)
          }
        }
      })()
    }, 140)

    return () => {
      window.clearTimeout(timer)
      abort.abort()
    }
  }, [open, value, fallbackPrefix, limit])

  // Hide the listbox on any pointer press outside the field (e.g. tapping a
  // frequent-directory chip or a dialog button), instead of relying solely on the
  // deferred blur, which some browsers (Safari/iOS) skip for non-focusable targets.
  // `focused` is left untouched so continued typing still works; instead we suppress
  // reopening until the next keystroke, so a value-driven refetch (e.g. from the
  // tapped chip) does not pop the listbox back up over the control just pressed.
  useEffect(() => {
    if (!focused) return
    function handlePointerDown(event: PointerEvent) {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setSuggestions([])
        setActiveSuggestion(-1)
        setSuppressed(true)
      }
    }
    document.addEventListener('pointerdown', handlePointerDown)
    return () => document.removeEventListener('pointerdown', handlePointerDown)
  }, [focused])

  // Cancel a pending deferred-blur so a refocus/selection within its 80ms window is
  // not clobbered (which would strand `focused=false` while the input still has focus).
  useEffect(
    () => () => {
      if (blurTimerRef.current !== undefined) {
        window.clearTimeout(blurTimerRef.current)
      }
    },
    [],
  )

  function cancelPendingBlur() {
    if (blurTimerRef.current !== undefined) {
      window.clearTimeout(blurTimerRef.current)
      blurTimerRef.current = undefined
    }
  }

  function selectSuggestion(next: string) {
    cancelPendingBlur()
    onChange(next)
    setSuggestions([])
    setActiveSuggestion(-1)
    setFocused(true)
    setSuppressed(false)
  }

  const expanded = open && focused && !suppressed && (loading || suggestions.length > 0)

  return (
    <div ref={containerRef} className="relative">
      <Input
        id={id}
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        placeholder={placeholder}
        className={className}
        value={value}
        role="combobox"
        tabIndex={0}
        aria-expanded={expanded}
        aria-autocomplete="list"
        aria-controls={expanded ? listboxId : undefined}
        aria-activedescendant={
          expanded && activeSuggestion >= 0 && activeSuggestion < suggestions.length
            ? `${listboxId}-option-${activeSuggestion}`
            : undefined
        }
        onChange={(event) => {
          onChange(event.target.value)
          setActiveSuggestion(-1)
          setSuppressed(false)
        }}
        onFocus={() => {
          cancelPendingBlur()
          setFocused(true)
          setSuppressed(false)
        }}
        onBlur={() => {
          cancelPendingBlur()
          blurTimerRef.current = window.setTimeout(() => {
            blurTimerRef.current = undefined
            setFocused(false)
            setActiveSuggestion(-1)
          }, 80)
        }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            setSuggestions([])
            setActiveSuggestion(-1)
            return
          }
          if (suggestions.length === 0) return

          if (event.key === 'ArrowDown') {
            event.preventDefault()
            setSuppressed(false)
            setActiveSuggestion((prev) => Math.min(prev + 1, suggestions.length - 1))
            return
          }
          if (event.key === 'ArrowUp') {
            event.preventDefault()
            setSuppressed(false)
            setActiveSuggestion((prev) => Math.max(prev - 1, 0))
            return
          }
          // Enter/Tab only act on a visible listbox: while suppressed/hidden, Tab must
          // move focus normally and Enter must reach the form, not select a hidden item.
          if (!expanded) return
          if (event.key === 'Enter' || event.key === 'Tab') {
            const idx = activeSuggestion
            if (idx >= 0 && idx < suggestions.length) {
              event.preventDefault()
              selectSuggestion(suggestions[idx])
              return
            }
            if (event.key === 'Tab' && suggestions.length === 1) {
              event.preventDefault()
              selectSuggestion(suggestions[0])
            }
          }
        }}
      />

      {expanded && (
        <div
          id={listboxId}
          role="listbox"
          className="absolute left-0 right-0 z-20 mt-1 max-h-44 overflow-auto rounded-md border border-border bg-popover p-1 shadow-md"
        >
          {loading && (
            <div className="px-2 py-1 text-[11px] text-secondary-foreground">
              Searching directories...
            </div>
          )}
          {!loading &&
            suggestions.map((item, idx) => (
              <button
                key={item}
                id={`${listboxId}-option-${idx}`}
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
  )
}
