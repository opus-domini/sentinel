import type { LucideIcon } from 'lucide-react'
import { Activity, BookOpen, Clock, Server, Settings } from 'lucide-react'

const SOURCE_ICON_MAP: Record<string, LucideIcon> = {
  runbook: BookOpen,
  service: Server,
  schedule: Clock,
  config: Settings,
}

export const ACTIVITY_SOURCES = ['runbook', 'service', 'schedule', 'config'] as const

export type ActivitySource = (typeof ACTIVITY_SOURCES)[number]

export function getActivitySourceIcon(source: string): LucideIcon {
  return SOURCE_ICON_MAP[source] ?? Activity
}
