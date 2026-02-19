import { useCallback, useState } from 'react'
import { QueryClient, useQueryClient } from '@tanstack/react-query'
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

type AuthCookieUpdateResult = {
  ok: boolean
  status: number
}

async function updateAuthCookie(
  queryClient: QueryClient,
  rawToken: string,
): Promise<AuthCookieUpdateResult> {
  const token = rawToken.trim()
  const headers: Record<string, string> = { Accept: 'application/json' }
  const request: RequestInit = {
    method: token === '' ? 'DELETE' : 'PUT',
    credentials: 'same-origin',
    headers,
  }
  if (token !== '') {
    headers['Content-Type'] = 'application/json'
    request.body = JSON.stringify({ token })
  }

  let response: Response | null = null
  try {
    response = await fetch('/api/auth/token', request)
  } catch {
    response = null
  }

  await queryClient.invalidateQueries({
    queryKey: ['meta'],
    exact: true,
  })
  await queryClient.refetchQueries({
    queryKey: ['meta'],
    exact: true,
    type: 'active',
  })

  return {
    ok: response?.ok === true,
    status: response?.status ?? 0,
  }
}

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
            This server requires a token. Access stays blocked until the token
            is validated.
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
                if (result.status === 401) {
                  setError('Invalid token.')
                  return
                }
                setError('Unable to validate token right now.')
              })
              .finally(() => {
                setSubmitting(false)
              })
          }}
        >
          <Input
            placeholder="token"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            autoFocus
            aria-label="Authentication token"
            disabled={submitting}
          />
          {error !== '' && (
            <p className="mt-2 text-xs text-destructive-foreground">{error}</p>
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
    <div className="grid h-screen place-items-center bg-background text-foreground">
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

  const meta = useSentinelMeta()
  const authenticated = !meta.tokenRequired || !meta.unauthorized
  const needsTokenGate = meta.loaded && meta.tokenRequired && meta.unauthorized
  const showOutlet = meta.loaded && !needsTokenGate

  const setToken = useCallback(
    (token: string) => {
      void updateAuthCookie(queryClient, token)
    },
    [queryClient],
  )

  const submitGateToken = useCallback(
    (token: string) => updateAuthCookie(queryClient, token),
    [queryClient],
  )

  return (
    <MetaContext.Provider value={meta}>
      <TokenContext.Provider value={{ authenticated, setToken }}>
        <ToastContext.Provider value={{ toasts, pushToast, dismissToast }}>
          <LayoutContext.Provider value={layout}>
            <TooltipProvider delayDuration={300}>
              <ErrorBoundary>
                {showOutlet ? <Outlet /> : <LoadingGate />}
                {needsTokenGate && (
                  <TokenGateDialog onSubmit={submitGateToken} />
                )}
                <ToastViewport toasts={toasts} onDismiss={dismissToast} />
              </ErrorBoundary>
            </TooltipProvider>
          </LayoutContext.Provider>
        </ToastContext.Provider>
      </TokenContext.Provider>
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
