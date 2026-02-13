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
  onCreate: (name: string, cwd: string) => void
}

export default function CreateSessionDialog({
  open,
  onOpenChange,
  defaultCwd,
  onCreate,
}: CreateSessionDialogProps) {
  const normalizedDefaultCwd = useMemo(() => defaultCwd.trim(), [defaultCwd])
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState(normalizedDefaultCwd)

  useEffect(() => {
    if (!open) {
      setCwd(normalizedDefaultCwd)
    }
  }, [normalizedDefaultCwd, open])

  function handleOpenChange(next: boolean) {
    if (!next) {
      setName('')
      setCwd(normalizedDefaultCwd)
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
            <Input
              placeholder="working directory"
              value={cwd}
              onChange={(e) => setCwd(e.target.value)}
            />
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
