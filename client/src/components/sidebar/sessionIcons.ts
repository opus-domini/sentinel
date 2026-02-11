import {
  Bot,
  Code,
  Database,
  FileCode,
  Globe,
  Server,
  Terminal,
  TerminalSquare,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export type SessionIconEntry = {
  key: string
  label: string
  icon: LucideIcon
}

export const SESSION_ICONS: Array<SessionIconEntry> = [
  { key: 'terminal', label: 'Terminal', icon: Terminal },
  { key: 'terminal-square', label: 'Shell', icon: TerminalSquare },
  { key: 'file-code', label: 'Editor', icon: FileCode },
  { key: 'bot', label: 'AI', icon: Bot },
  { key: 'code', label: 'Code', icon: Code },
  { key: 'server', label: 'Server', icon: Server },
  { key: 'database', label: 'Database', icon: Database },
  { key: 'globe', label: 'Web', icon: Globe },
]

const iconMap = new Map(SESSION_ICONS.map((e) => [e.key, e.icon]))
export const DEFAULT_ICON_KEY = 'terminal'

export function getSessionIcon(key: string): LucideIcon {
  return iconMap.get(key) || iconMap.get(DEFAULT_ICON_KEY)!
}
