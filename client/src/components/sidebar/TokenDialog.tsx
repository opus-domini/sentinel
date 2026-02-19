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
  token: string
  onTokenChange: (value: string) => void
  tokenRequired: boolean
}

export default function TokenDialog({
  open,
  onOpenChange,
  token,
  onTokenChange,
  tokenRequired,
}: TokenDialogProps) {
  const [draft, setDraft] = useState(token)

  function handleOpenChange(next: boolean) {
    if (next) {
      setDraft(token)
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
          <DialogTitle>API token</DialogTitle>
          <DialogDescription>
            {tokenRequired
              ? 'A token is required to access the server.'
              : 'Set a bearer token for API authentication.'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <Input
            placeholder={
              tokenRequired ? 'token (required)' : 'token (optional)'
            }
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            autoFocus
          />
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>
            <Button type="submit">Save</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
