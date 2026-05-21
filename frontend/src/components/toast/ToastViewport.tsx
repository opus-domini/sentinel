import { X } from 'lucide-react'
import type { ToastMessage } from '../../hooks/useToasts'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ToastViewportProps = {
  toasts: Array<ToastMessage>
  onDismiss: (id: number) => void
}

export default function ToastViewport({
  toasts,
  onDismiss,
}: ToastViewportProps) {
  if (toasts.length === 0) {
    return null
  }

  return (
    <div
      aria-live="polite"
      role="status"
      className="pointer-events-none fixed bottom-3 right-3 z-50 grid w-[min(420px,calc(100vw-24px))] gap-2"
    >
      {toasts.map((toast) => (
        <article
          key={toast.id}
          className={cn(
            'pointer-events-auto rounded-lg border bg-surface-raised p-3 shadow-[0_10px_30px_rgba(0,0,0,.35)]',
            toast.level === 'success' && 'border-ok/45',
            toast.level === 'error' && 'border-destructive/55',
            toast.level === 'info' && 'border-primary/45',
          )}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="grid gap-1">
              <strong className="text-[12px]">{toast.title}</strong>
              <p className="m-0 text-[12px] text-secondary-foreground">
                {toast.message}
              </p>
            </div>
            <Button
              variant="ghost"
              size="icon-xs"
              className="shrink-0 text-muted-foreground"
              onClick={() => onDismiss(toast.id)}
              aria-label="Dismiss notification"
            >
              <X />
            </Button>
          </div>
        </article>
      ))}
    </div>
  )
}
