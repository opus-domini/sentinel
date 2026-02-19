import type { TerminalTheme } from '@/lib/terminalThemes'

import { terminalThemes } from '@/lib/terminalThemes'
import { cn } from '@/lib/utils'

type ThemeSelectorProps = {
  activeThemeId: string
  onSelect: (id: string) => void
}

const ansiKeys = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
] as const

function ThemeCard({
  theme,
  isActive,
  onSelect,
}: {
  theme: TerminalTheme
  isActive: boolean
  onSelect: () => void
}) {
  const colors = theme.colors

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'flex flex-col gap-2 rounded-lg border p-3 text-left transition-colors',
        isActive
          ? 'border-primary/60 ring-2 ring-primary'
          : 'border-border hover:border-border-bright',
      )}
    >
      <div
        className="flex h-16 w-full items-end rounded-md px-2 pb-2"
        style={{ backgroundColor: colors.background ?? '#000' }}
      >
        <div className="flex gap-1">
          {ansiKeys.map((key) => (
            <span
              key={key}
              className="inline-block h-2.5 w-2.5 rounded-full"
              style={{
                backgroundColor: colors[key] ?? '#888',
              }}
            />
          ))}
        </div>
      </div>
      <span className="text-sm text-foreground">{theme.label}</span>
    </button>
  )
}

export default function ThemeSelector({
  activeThemeId,
  onSelect,
}: ThemeSelectorProps) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {terminalThemes.map((theme) => (
        <ThemeCard
          key={theme.id}
          theme={theme}
          isActive={theme.id === activeThemeId}
          onSelect={() => onSelect(theme.id)}
        />
      ))}
    </div>
  )
}
