import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Outlet, createRootRoute, useRouterState } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { ServerOfflineBanner } from '@/components/ServerOfflineBanner'
import { ConnectionIssueBanner } from '@/components/ConnectionIssueBanner'
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
import { ConnectionHealthContext } from '@/contexts/ConnectionHealthContext'
import { MetaContext } from '@/contexts/MetaContext'
import { OpsEventsContext } from '@/contexts/OpsEventsContext'
import { ToastContext } from '@/contexts/ToastContext'
import { TokenContext } from '@/contexts/TokenContext'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { useConnectionCheck } from '@/hooks/useConnectionCheck'
import { useSentinelMeta } from '@/hooks/useSentinelMeta'
import { useServerStatus } from '@/hooks/useServerStatus'
import { useSharedOpsEventsSocket } from '@/hooks/useSharedOpsEventsSocket'
import { useShellLayout } from '@/hooks/useShellLayout'
import { useToasts } from '@/hooks/useToasts'
import { useVisualViewport } from '@/hooks/useVisualViewport'
import { applyDocumentAppBrand } from '@/lib/appBrand'
import { authCookieUpdateErrorMessage, updateAuthCookie } from '@/lib/authToken'
import type { AuthCookieUpdateResult } from '@/lib/authToken'

function TokenGateDialog({
  onSubmit,
}: {
  onSubmit: (token: string) => Promise<AuthCookieUpdateResult>
}) {
  const [draft, setDraft] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  return (
    <Dialog open>
      <DialogContent
        showCloseButton={false}
        onInteractOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Authentication required</DialogTitle>
          <DialogDescription>
            This server requires a token. Access stays blocked until the token is validated.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            const token = draft.trim()
            if (token === '' || submitting) return
            setSubmitting(true)
            setError('')
            void onSubmit(token)
              .then((result) => {
                if (result.ok) {
                  setDraft('')
                  return
                }
                setError(authCookieUpdateErrorMessage(result))
              })
              .finally(() => {
                setSubmitting(false)
              })
          }}
        >
          <Input
            name="auth-token"
            placeholder="token"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            aria-label="Authentication token"
            disabled={submitting}
          />
          {error !== '' && (
            <p role="alert" className="mt-2 text-xs text-destructive-foreground">
              {error}
            </p>
          )}
          <DialogFooter className="mt-4">
            <Button type="submit" disabled={draft.trim() === '' || submitting}>
              Continue
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function LoadingGate() {
  return (
    <div className="grid h-dvh place-items-center bg-background text-foreground">
      <div className="text-center">
        <p className="text-sm text-secondary-foreground">Loading Sentinel...</p>
      </div>
    </div>
  )
}

function RootComponent() {
  useVisualViewport()
  const { toasts, pushToast, dismissToast } = useToasts()
  const queryClient = useQueryClient()
  const layout = useShellLayout({
    storageKey: 'sentinel_sidebar_collapsed',
    defaultSidebarWidth: 340,
    minSidebarWidth: 240,
    maxSidebarWidth: 440,
    onResizeEnd: () => {
      window.dispatchEvent(new Event('resize'))
    },
  })
  const { setSidebarOpen } = layout

  const { offline, retry } = useServerStatus()
  const meta = useSentinelMeta()
  const isMobile = useIsMobileLayout()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  useEffect(() => {
    applyDocumentAppBrand(meta.hostname)
  }, [meta.hostname])

  useEffect(() => {
    if (isMobile) {
      setSidebarOpen(false)
    }
  }, [isMobile, pathname, setSidebarOpen])

  const authenticated = !meta.tokenRequired || !meta.unauthorized
  const needsTokenGate = meta.loaded && meta.tokenRequired && meta.unauthorized

  const handlePreflightUnauthorized = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['meta'], exact: true })
  }, [queryClient])
  const connectionHealth = useConnectionCheck({
    enabled: meta.loaded && authenticated,
    onUnauthorized: handlePreflightUnauthorized,
  })
  const showOutlet = meta.loaded && !needsTokenGate && connectionHealth.ready

  const opsEvents = useSharedOpsEventsSocket({
    authenticated,
    tokenRequired: meta.tokenRequired,
    connectionReady: connectionHealth.ready,
  })

  const setToken = useCallback(
    (token: string) => {
      return updateAuthCookie(queryClient, token)
    },
    [queryClient],
  )

  const submitGateToken = useCallback(
    (token: string) => updateAuthCookie(queryClient, token),
    [queryClient],
  )

  const tokenContextValue = useMemo(() => ({ authenticated, setToken }), [authenticated, setToken])
  const toastContextValue = useMemo(
    () => ({ toasts, pushToast, dismissToast }),
    [dismissToast, pushToast, toasts],
  )

  return (
    <MetaContext.Provider value={meta}>
      <TokenContext.Provider value={tokenContextValue}>
        <ToastContext.Provider value={toastContextValue}>
          <ConnectionHealthContext.Provider value={connectionHealth}>
            <OpsEventsContext.Provider value={opsEvents}>
              <LayoutContext.Provider value={layout}>
                <TooltipProvider delayDuration={300}>
                  <ErrorBoundary>
                    {showOutlet ? <Outlet /> : <LoadingGate />}
                    {needsTokenGate && <TokenGateDialog onSubmit={submitGateToken} />}
                    <ToastViewport toasts={toasts} onDismiss={dismissToast} />
                    {!offline && connectionHealth.issue && (
                      <ConnectionIssueBanner
                        issue={connectionHealth.issue}
                        checking={connectionHealth.checking}
                        onRetry={connectionHealth.retry}
                      />
                    )}
                    {offline && <ServerOfflineBanner onRetry={retry} />}
                  </ErrorBoundary>
                </TooltipProvider>
              </LayoutContext.Provider>
            </OpsEventsContext.Provider>
          </ConnectionHealthContext.Provider>
        </ToastContext.Provider>
      </TokenContext.Provider>
    </MetaContext.Provider>
  )
}

function NotFoundComponent() {
  return (
    <div className="grid h-dvh place-items-center bg-background text-foreground">
      <div className="text-center">
        <h1 className="text-2xl font-bold">404</h1>
        <p className="mt-2 text-secondary-foreground">Page not found</p>
        <a href="/tmux" className="mt-4 inline-block text-primary hover:underline">
          Go to tmux
        </a>
      </div>
    </div>
  )
}

function ErrorComponent({ error }: { error: Error }) {
  return (
    <div className="grid h-dvh place-items-center bg-background text-foreground">
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
