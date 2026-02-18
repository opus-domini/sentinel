import { useEffect, useState } from 'react'
import { Outlet, createRootRoute } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import ToastViewport from '@/components/toast/ToastViewport'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { TooltipProvider } from '@/components/ui/tooltip'
import { LayoutContext } from '@/contexts/LayoutContext'
import { MetaContext } from '@/contexts/MetaContext'
import { ToastContext } from '@/contexts/ToastContext'
import { TokenContext } from '@/contexts/TokenContext'
import { useSentinelMeta } from '@/hooks/useSentinelMeta'
import { useShellLayout } from '@/hooks/useShellLayout'
import { useToasts } from '@/hooks/useToasts'
import { useVisualViewport } from '@/hooks/useVisualViewport'

function TokenGateDialog({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [draft, setDraft] = useState('')

  return (
    <Dialog open>
      <DialogContent
        showCloseButton={false}
        onInteractOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Authentication required</DialogTitle>
          <DialogDescription>
            This server requires a token. Enter it to continue.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (draft.trim()) onSubmit(draft.trim())
          }}
        >
          <Input
            placeholder="token"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            autoFocus
            aria-label="Authentication token"
          />
          <DialogFooter className="mt-4">
            <Button type="submit" disabled={!draft.trim()}>
              Continue
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RootComponent() {
  useVisualViewport()
  const { toasts, pushToast, dismissToast } = useToasts()
  const layout = useShellLayout({
    storageKey: 'sentinel_sidebar_collapsed',
    defaultSidebarWidth: 340,
    minSidebarWidth: 240,
    maxSidebarWidth: 440,
    onResizeEnd: () => {
      window.dispatchEvent(new Event('resize'))
    },
  })

  const [token, setToken] = useState(
    () => window.localStorage.getItem('sentinel_token') ?? '',
  )
  const meta = useSentinelMeta(token)

  useEffect(() => {
    window.localStorage.setItem('sentinel_token', token)
  }, [token])

  return (
    <MetaContext.Provider value={meta}>
      <TokenContext.Provider value={{ token, setToken }}>
        <ToastContext.Provider value={{ toasts, pushToast, dismissToast }}>
          <LayoutContext.Provider value={layout}>
            <TooltipProvider delayDuration={300}>
              <ErrorBoundary>
                <Outlet />
              </ErrorBoundary>
              <ToastViewport toasts={toasts} onDismiss={dismissToast} />
            </TooltipProvider>
          </LayoutContext.Provider>
        </ToastContext.Provider>
      </TokenContext.Provider>
      {meta.tokenRequired && (!token.trim() || meta.unauthorized) && (
        <TokenGateDialog onSubmit={setToken} />
      )}
    </MetaContext.Provider>
  )
}

function NotFoundComponent() {
  return (
    <div className="grid h-screen place-items-center bg-background text-foreground">
      <div className="text-center">
        <h1 className="text-2xl font-bold">404</h1>
        <p className="mt-2 text-secondary-foreground">Page not found</p>
        <a
          href="/tmux"
          className="mt-4 inline-block text-primary hover:underline"
        >
          Go to tmux
        </a>
      </div>
    </div>
  )
}

function ErrorComponent({ error }: { error: Error }) {
  return (
    <div className="grid h-screen place-items-center bg-background text-foreground">
      <div className="text-center">
        <h1 className="text-2xl font-bold text-destructive">Error</h1>
        <p className="mt-2 text-secondary-foreground">{error.message}</p>
      </div>
    </div>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
  errorComponent: ErrorComponent,
})
