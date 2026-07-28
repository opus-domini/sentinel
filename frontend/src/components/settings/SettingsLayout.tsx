import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { ArrowLeft, SlidersHorizontal } from 'lucide-react'

import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import SettingsNavigation from './SettingsNavigation'
import { settingsSectionFromPath } from './settingsSections'
import { Button } from '@/components/ui/button'
import { useMetaContext } from '@/contexts/MetaContext'

export default function SettingsLayout() {
  const { hostname } = useMetaContext()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const activeSection = settingsSectionFromPath(pathname)

  return (
    <AppShell>
      <main className="grid h-full min-h-0 min-w-0 grid-rows-[44px_1fr] bg-[radial-gradient(circle_at_12%_-8%,var(--section-glow-brand),transparent_32%),radial-gradient(circle_at_92%_8%,rgba(114,245,187,0.06),transparent_24%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <AppSectionTitle hostname={hostname} section="settings" />
          </div>
          <Button asChild variant="ghost" className="min-h-11">
            <Link to="/" aria-label="Back to Now">
              <ArrowLeft className="size-3.5" aria-hidden="true" />
              <span className="hidden sm:inline">Now</span>
            </Link>
          </Button>
        </header>

        <div
          data-testid="settings-scroll-owner"
          className="min-h-0 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain"
        >
          <div className="mx-auto grid w-full max-w-6xl gap-4 p-3 pb-6 sm:p-4 md:gap-5 md:grid-cols-[15rem_minmax(0,1fr)] md:py-6">
            <div className="grid min-w-0 content-start gap-4 md:sticky md:top-6 md:self-start">
              <div className="flex items-start gap-3 px-1">
                <span className="grid size-9 shrink-0 place-items-center rounded-lg border border-primary/20 bg-primary/10 text-primary-text">
                  <SlidersHorizontal className="size-4" aria-hidden="true" />
                </span>
                <div>
                  <p className="text-sm font-semibold">Local control plane</p>
                  <p className="mt-1 text-[10px] leading-relaxed text-muted-foreground">
                    Shape this Sentinel deployment without leaving the daily workspace.
                  </p>
                </div>
              </div>
              <SettingsNavigation activeSection={activeSection} />
            </div>

            <section
              aria-label={`${activeSection} settings`}
              className="min-w-0 rounded-xl border border-border-subtle bg-surface-raised/80 p-3 shadow-[0_18px_60px_rgba(0,0,0,0.22)] sm:p-4 md:min-h-[32rem] md:p-5"
            >
              <Outlet />
            </section>
          </div>
        </div>
      </main>
    </AppShell>
  )
}
