import type { LucideIcon } from 'lucide-react'
import {
  Bot,
  Braces,
  Bug,
  Cloud,
  Database,
  Globe,
  Server,
  TerminalSquare,
} from 'lucide-react'

export type TmuxIconEntry = {
  key: string
  label: string
  icon: LucideIcon
}

export const TMUX_ICONS: Array<TmuxIconEntry> = [
  { key: 'bot', label: 'AI', icon: Bot },
  { key: 'debug', label: 'Debug', icon: Bug },
  { key: 'code', label: 'Code', icon: Braces },
  { key: 'cloud', label: 'Cloud', icon: Cloud },
  { key: 'database', label: 'Database', icon: Database },
  { key: 'globe', label: 'Web', icon: Globe },
  { key: 'server', label: 'Server', icon: Server },
  { key: 'terminal', label: 'Terminal', icon: TerminalSquare },
]

const iconMap = new Map(TMUX_ICONS.map((entry) => [entry.key, entry.icon]))

export const DEFAULT_ICON_KEY = 'terminal'

export function getTmuxIcon(key: string): LucideIcon {
  return iconMap.get(key) || iconMap.get(DEFAULT_ICON_KEY)!
}
