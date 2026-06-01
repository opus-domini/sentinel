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

type RenameDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  value: string
  onValueChange: (value: string) => void
  onSubmit: () => Promise<void>
  onClose?: () => void
}

export default function RenameDialog(props: RenameDialogProps) {
  const { open, onOpenChange, title, description, value, onValueChange, onSubmit, onClose } = props
  const id = useId()
  const inputId = `${id}-name`
  const errorId = `${id}-error`
  const inputRef = useRef<HTMLInputElement>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setError('')
    window.setTimeout(() => inputRef.current?.focus(), 0)
  }, [open])

  function closeDialog() {
    onOpenChange(false)
    setSaving(false)
    setError('')
    if (onClose) onClose()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) {
          onOpenChange(true)
          return
        }
        if (saving) return
        closeDialog()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={async (e) => {
            e.preventDefault()
            if (saving || value.trim() === '') return
            setSaving(true)
            setError('')
            try {
              await onSubmit()
              closeDialog()
            } catch (err) {
              setError(err instanceof Error ? err.message : 'Rename failed')
              setSaving(false)
            }
          }}
        >
          <label
            htmlFor={inputId}
            className="mb-1 block text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground"
          >
            New name
          </label>
          <Input
            id={inputId}
            ref={inputRef}
            value={value}
            disabled={saving}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
            onChange={(e) => {
              onValueChange(e.target.value)
              setError('')
            }}
          />
          {error !== '' && (
            <p id={errorId} role="alert" className="mt-2 text-xs text-destructive-foreground">
              {error}
            </p>
          )}
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline" disabled={saving}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={saving || value.trim() === ''}>
              Rename
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
