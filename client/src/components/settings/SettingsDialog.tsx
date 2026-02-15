import { useState } from 'react'

import ThemeSelector from '@/components/settings/ThemeSelector'
import { useMetaContext } from '@/contexts/MetaContext'
import { usePwaInstall } from '@/hooks/usePwaInstall'
import { cn } from '@/lib/utils'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { THEME_STORAGE_KEY } from '@/lib/terminalThemes'

type SettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function SettingsDialog({
  open,
  onOpenChange,
}: SettingsDialogProps) {
  const { version } = useMetaContext()
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )
  const {
    supportsPwa,
    installed,
    installAvailable,
    installApp,
    updateAvailable,
    applyUpdate,
    updating,
  } = usePwaInstall()

  const selectTheme = (id: string) => {
    setThemeId(id)
    localStorage.setItem(THEME_STORAGE_KEY, id)
    window.dispatchEvent(
      new CustomEvent('sentinel-theme-change', { detail: id }),
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
          <DialogDescription>
            Configure your Sentinel experience.
          </DialogDescription>
        </DialogHeader>
        <section>
          <h3 className="mb-1 text-xs font-medium">Terminal Theme</h3>
          <p className="mb-3 text-xs text-muted-foreground">
            Choose a color theme for the terminal emulator.
          </p>
          <ThemeSelector activeThemeId={themeId} onSelect={selectTheme} />
        </section>

        <section className="mt-4 border-t border-border-subtle pt-4">
          <div className="mb-1 flex items-center gap-2">
            <h3 className="text-xs font-medium">Progressive App</h3>
            <Badge
              variant="outline"
              className={cn(
                installed
                  ? 'border-ok/45 bg-ok/10 text-ok-foreground'
                  : 'border-border-subtle bg-surface-overlay text-muted-foreground',
              )}
            >
              {installed ? 'Installed' : 'Browser'}
            </Badge>
          </div>
          <p className="mb-3 text-xs text-muted-foreground">
            Install Sentinel as an app for faster launch and better mobile UX.
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              size="sm"
              onClick={() => {
                void installApp()
              }}
              disabled={!installAvailable}
              title={
                installAvailable
                  ? 'Install Sentinel'
                  : 'Use browser install menu when available'
              }
            >
              Install App
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={applyUpdate}
              disabled={!updateAvailable || updating}
            >
              {updating ? 'Updating...' : 'Apply Update'}
            </Button>
            {!supportsPwa && (
              <span className="text-[11px] text-warning-foreground">
                PWA needs HTTPS or localhost.
              </span>
            )}
          </div>
        </section>

        <section className="mt-4 border-t border-border-subtle pt-4">
          <h3 className="mb-1 text-xs font-medium">Sentinel</h3>
          <p className="text-xs text-muted-foreground">
            Version: <span className="font-mono">{version || 'dev'}</span>
          </p>
        </section>
      </DialogContent>
    </Dialog>
  )
}
