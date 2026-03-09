import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  Bell,
  BookOpen,
  Clock,
  Server,
  Settings,
  Shield,
} from 'lucide-react'

const SOURCE_ICON_MAP: Record<string, LucideIcon> = {
  runbook: BookOpen,
  service: Server,
  alert: Bell,
  guardrail: Shield,
  schedule: Clock,
  config: Settings,
}

export const ACTIVITY_SOURCES = [
  'runbook',
  'service',
  'alert',
  'guardrail',
  'schedule',
  'config',
] as const

export type ActivitySource = (typeof ACTIVITY_SOURCES)[number]

export function getActivitySourceIcon(source: string): LucideIcon {
  return SOURCE_ICON_MAP[source] ?? Activity
}
