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

type RenameDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  value: string
  onValueChange: (value: string) => void
  onSubmit: () => void | Promise<void>
  onClose?: () => void
}

export default function RenameDialog(props: RenameDialogProps) {
  const {
    open,
    onOpenChange,
    title,
    description,
    value,
    onValueChange,
    onSubmit,
    onClose,
  } = props
  const [saving, setSaving] = useState(false)

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen) {
          setSaving(false)
          if (onClose) onClose()
        }
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
            if (saving) return
            setSaving(true)
            try {
              await onSubmit()
            } finally {
              setSaving(false)
            }
          }}
        >
          <Input
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            autoFocus
          />
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>
            <Button type="submit" disabled={saving}>
              Rename
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
