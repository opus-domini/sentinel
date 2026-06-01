import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

type DismissAlertDialogProps = {
  alertTitle: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export default function DismissAlertDialog({
  alertTitle,
  open,
  onOpenChange,
  onConfirm,
}: DismissAlertDialogProps) {
  const target = alertTitle?.trim() || 'this alert'

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Dismiss alert?</AlertDialogTitle>
          <AlertDialogDescription>
            This will dismiss <span className="font-medium text-foreground">{target}</span> from the
            resolved alerts list.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Dismiss
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
