export type Session = {
  name: string
  windows: number
  panes: number
  attached: number
  createdAt: string
  activityAt: string
  command: string
  hash: string
  lastContent: string
  icon: string
}

export type ConnectionState =
  | 'connected'
  | 'connecting'
  | 'disconnected'
  | 'error'

export type SessionsResponse = {
  sessions: Array<Session>
}

export type WindowInfo = {
  session: string
  index: number
  name: string
  active: boolean
  panes: number
}

export type PaneInfo = {
  session: string
  windowIndex: number
  paneIndex: number
  paneId: string
  title: string
  active: boolean
  tty: string
}

export type WindowsResponse = {
  windows: Array<WindowInfo>
}

export type PanesResponse = {
  panes: Array<PaneInfo>
}

export type TerminalConnection = {
  id: string
  tty: string
  user: string
  processCount: number
  leaderPid: number
  command: string
  args: string
}

export type TerminalsResponse = {
  terminals: Array<TerminalConnection>
}

export type TerminalProcess = {
  pid: number
  ppid: number
  user: string
  command: string
  args: string
  cpu: number
  mem: number
}

export type SystemTerminalDetailResponse = {
  tty: string
  processes: Array<TerminalProcess>
}
