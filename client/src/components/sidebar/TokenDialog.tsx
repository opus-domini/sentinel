import { useState } from 'react'
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

type TokenDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  authenticated: boolean
  onTokenChange: (value: string) => void
  tokenRequired: boolean
}

export default function TokenDialog({
  open,
  onOpenChange,
  authenticated,
  onTokenChange,
  tokenRequired,
}: TokenDialogProps) {
  const [draft, setDraft] = useState('')

  function handleOpenChange(next: boolean) {
    if (next) {
      setDraft('')
    }
    onOpenChange(next)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    onTokenChange(draft)
    onOpenChange(false)
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
            <p className="mb-3 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-2 text-[11px] text-emerald-200">
              Authenticated
            </p>
          )}
          {!authenticated && (
            <Input
              placeholder={tokenRequired ? 'token (required)' : 'token'}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              autoFocus
            />
          )}
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">
                {authenticated ? 'Close' : 'Cancel'}
              </Button>
            </DialogClose>
            {authenticated ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  onTokenChange('')
                  onOpenChange(false)
                }}
              >
                Clear cookie
              </Button>
            ) : (
              <Button type="submit" disabled={!draft.trim()}>
                Save
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
