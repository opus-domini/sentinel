import { WifiOff } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function ServerOfflineBanner({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-background/80">
      <div className="flex flex-col items-center gap-4 rounded-lg border bg-background p-8 shadow-lg">
        <WifiOff className="h-10 w-10 text-muted-foreground" />
        <div className="text-center">
          <h2 className="text-lg font-semibold">Server unreachable</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Sentinel is not responding. Check that the server is running.
          </p>
        </div>
        <Button variant="outline" onClick={onRetry}>
          Retry
        </Button>
      </div>
    </div>
  )
}
