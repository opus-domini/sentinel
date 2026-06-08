import { useEffect, useId, useRef, useState } from 'react'
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
import { authCookieUpdateErrorMessage } from '@/lib/authToken'
import type { AuthCookieUpdateResult } from '@/lib/authToken'

type TokenDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  authenticated: boolean
  onTokenChange: (value: string) => Promise<AuthCookieUpdateResult>
  tokenRequired: boolean
}

export default function TokenDialog({
  open,
  onOpenChange,
  authenticated,
  onTokenChange,
  tokenRequired,
}: TokenDialogProps) {
  const id = useId()
  const inputId = `${id}-token`
  const errorId = `${id}-error`
  const inputRef = useRef<HTMLInputElement>(null)
  const [draft, setDraft] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open || authenticated) return
    window.setTimeout(() => inputRef.current?.focus(), 0)
  }, [authenticated, open])

  function handleOpenChange(next: boolean) {
    if (!next && submitting) return
    if (next) {
      setDraft('')
      setError('')
    }
    onOpenChange(next)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const token = draft.trim()
    if (token === '' || submitting) return
    setSubmitting(true)
    setError('')
    try {
      const result = await onTokenChange(token)
      if (result.ok) {
        setDraft('')
        onOpenChange(false)
        return
      }
      setError(authCookieUpdateErrorMessage(result))
    } catch {
      setError('Unable to validate token right now.')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleClearToken() {
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      const result = await onTokenChange('')
      if (result.ok) {
        onOpenChange(false)
        return
      }
      setError(authCookieUpdateErrorMessage(result, { action: 'clear' }))
    } catch {
      setError('Unable to clear token right now.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Authentication token</DialogTitle>
          <DialogDescription>
            {tokenRequired
              ? 'This server requires a token. The value is stored in an HttpOnly cookie.'
              : 'Set a token in an HttpOnly cookie for this browser session.'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          {authenticated && (
            <p className="mb-3 rounded-md border border-ok/40 bg-ok/10 px-2.5 py-2 text-[11px] text-ok-foreground">
              Authenticated
            </p>
          )}
          {!authenticated && (
            <div>
              <label
                htmlFor={inputId}
                className="mb-1 block text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground"
              >
                Authentication token
              </label>
              <Input
                id={inputId}
                ref={inputRef}
                name="auth-token"
                placeholder={tokenRequired ? 'token (required)' : 'token'}
                value={draft}
                onChange={(e) => {
                  setDraft(e.target.value)
                  setError('')
                }}
                disabled={submitting}
                aria-invalid={error ? true : undefined}
                aria-describedby={error ? errorId : undefined}
              />
            </div>
          )}
          {error !== '' && (
            <p id={errorId} role="alert" className="mt-2 text-xs text-destructive-foreground">
              {error}
            </p>
          )}
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline" disabled={submitting}>
                {authenticated ? 'Close' : 'Cancel'}
              </Button>
            </DialogClose>
            {authenticated ? (
              <Button
                type="button"
                variant="outline"
                disabled={submitting}
                onClick={() => void handleClearToken()}
              >
                Clear cookie
              </Button>
            ) : (
              <Button type="submit" disabled={!draft.trim() || submitting}>
                Save
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
