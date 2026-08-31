import { Trash2 } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

type LauncherDialogActionsProps = {
  deleteTitle: string
  canDelete: boolean
  saving: boolean
  onDelete: () => void
  onSave: () => void
}

/** Delete-with-confirmation plus Save footer shared by the launcher dialogs. */
export default function LauncherDialogActions({
  deleteTitle,
  canDelete,
  saving,
  onDelete,
  onSave,
}: LauncherDialogActionsProps) {
  return (
    <div className="mt-auto flex flex-wrap items-center gap-2">
      <div className="ml-auto flex items-center gap-2">
        {canDelete && (
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button type="button" variant="destructive" size="sm" className="cursor-pointer">
                <Trash2 className="h-3.5 w-3.5" />
                Delete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{deleteTitle}</AlertDialogTitle>
                <AlertDialogDescription>This action cannot be undone.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction variant="destructive" onClick={onDelete}>
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        )}
        <Button
          type="button"
          size="sm"
          className="cursor-pointer"
          onClick={onSave}
          disabled={saving}
        >
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
