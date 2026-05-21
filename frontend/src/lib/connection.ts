import type { ConnectionState } from '../types'

export function connectionBadgeClass(state: ConnectionState): string {
  switch (state) {
    case 'connected':
      return 'border-ok/45 bg-ok/20 text-ok-foreground'
    case 'connecting':
      return 'border-warning/45 bg-warning/20 text-warning-foreground'
    case 'error':
      return 'border-destructive/45 bg-destructive/20 text-destructive-foreground'
    default:
      return 'border-muted-foreground/35 bg-muted-foreground/20 text-disconnected-text'
  }
}

export function connectionDotClass(state: ConnectionState): string {
  switch (state) {
    case 'connected':
      return 'bg-ok'
    case 'connecting':
      return 'bg-warning'
    case 'error':
      return 'bg-destructive'
    default:
      return 'bg-muted-foreground'
  }
}

export function connectionLabel(state: ConnectionState): string {
  switch (state) {
    case 'connected':
      return 'Connected'
    case 'connecting':
      return 'Connecting'
    case 'disconnected':
      return 'Disconnected'
    case 'error':
      return 'Error'
  }
}
