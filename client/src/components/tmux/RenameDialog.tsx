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

type RenameDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  value: string
  onValueChange: (value: string) => void
  onSubmit: () => void
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

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen && onClose) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit()
          }}
        >
          <Input
            value={value}
            onChange={(e) => onValueChange(slugifyTmuxName(e.target.value))}
            autoFocus
          />
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>
            <Button type="submit">Rename</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
