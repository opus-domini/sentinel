import { Activity, Blocks, ScrollText, SquareTerminal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export type PrimaryNavItem = {
  to: '/tmux' | '/metrics' | '/services' | '/runbooks'
  label: string
  // shortLabel is used in width-constrained nav (the mobile bottom bar). Falls back to label.
  shortLabel?: string
  Icon: LucideIcon
}

export const PRIMARY_NAV_ITEMS: Array<PrimaryNavItem> = [
  {
    to: '/tmux',
    label: 'Tmux',
    Icon: SquareTerminal,
  },
  {
    to: '/services',
    label: 'Services',
    Icon: Blocks,
  },
  {
    to: '/runbooks',
    label: 'Runbooks',
    Icon: ScrollText,
  },
  {
    to: '/metrics',
    label: 'Metrics',
    Icon: Activity,
  },
]
