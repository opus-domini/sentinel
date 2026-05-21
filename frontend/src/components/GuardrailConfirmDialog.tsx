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
import { Badge } from '@/components/ui/badge'

type GuardrailConfirmDialogProps = {
  open: boolean
  ruleName: string
  message: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export default function GuardrailConfirmDialog({
  open,
  ruleName,
  message,
  onOpenChange,
  onConfirm,
}: GuardrailConfirmDialogProps) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onOpenChange(false)
      }}
    >
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Guardrail Confirmation</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              {ruleName.trim() !== '' && (
                <Badge variant="outline" className="text-[11px]">
                  {ruleName}
                </Badge>
              )}
              <p>{message}</p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Confirm
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
