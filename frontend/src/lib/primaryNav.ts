import { Activity, Bell, Blocks, Clock, ScrollText, SquareTerminal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export type PrimaryNavItem = {
  to: '/tmux' | '/metrics' | '/services' | '/alerts' | '/runbooks' | '/activities'
  label: string
  Icon: LucideIcon
}

export const PRIMARY_NAV_ITEMS: Array<PrimaryNavItem> = [
  {
    to: '/tmux',
    label: 'Tmux',
    Icon: SquareTerminal,
  },
  {
    to: '/metrics',
    label: 'Metrics',
    Icon: Activity,
  },
  {
    to: '/services',
    label: 'Services',
    Icon: Blocks,
  },
  {
    to: '/alerts',
    label: 'Alerts',
    Icon: Bell,
  },
  {
    to: '/runbooks',
    label: 'Runbooks',
    Icon: ScrollText,
  },
  {
    to: '/activities',
    label: 'Activities',
    Icon: Clock,
  },
]
