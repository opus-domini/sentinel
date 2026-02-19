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

type KillSessionDialogProps = {
  session: string | null
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export default function KillSessionDialog(props: KillSessionDialogProps) {
  const { session, onOpenChange, onConfirm } = props

  return (
    <AlertDialog
      open={session !== null}
      onOpenChange={(open) => {
        if (!open) onOpenChange(false)
      }}
    >
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Kill session</AlertDialogTitle>
          <AlertDialogDescription>
            Kill session {session}? This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Kill
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
