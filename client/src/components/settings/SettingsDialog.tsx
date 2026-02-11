import { useState } from 'react'

import ThemeSelector from '@/components/settings/ThemeSelector'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { THEME_STORAGE_KEY } from '@/lib/terminalThemes'

type SettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function SettingsDialog({
  open,
  onOpenChange,
}: SettingsDialogProps) {
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )

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
      </DialogContent>
    </Dialog>
  )
}
