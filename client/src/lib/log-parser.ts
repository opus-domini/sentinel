export type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'notice' | 'unknown'

export type ParsedLogLine = {
  lineNumber: number
  raw: string
  timestamp: string
  hostname: string
  unit: string
  message: string
  level: LogLevel
}

const journalctlRegex =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{4})\s+(\S+)\s+(\S+(?:\[\d+\])?):\s+(.*)/

const launchdRegex =
  /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+[+-]\d{4})\s+(\S+)\s+(\S+):\s+(.*)/

const errorPattern = /\b(?:ERROR|FATAL|CRIT|CRITICAL|PANIC)\b/i
const warnPattern = /\b(?:WARN|WARNING)\b/i
const infoPattern = /\bINFO\b/i
const debugPattern = /\b(?:DEBUG|TRACE)\b/i
const noticePattern = /\bNOTICE\b/i

const levelKeyValuePattern = /\b(?:level|severity)=(\S+)/i

export function detectLevel(message: string): LogLevel {
  const kvMatch = message.match(levelKeyValuePattern)
  if (kvMatch) {
    const value = kvMatch[1].toLowerCase()
    if (
      value === 'error' ||
      value === 'fatal' ||
      value === 'crit' ||
      value === 'critical' ||
      value === 'panic'
    )
      return 'error'
    if (value === 'warn' || value === 'warning') return 'warn'
    if (value === 'info') return 'info'
    if (value === 'debug' || value === 'trace') return 'debug'
    if (value === 'notice') return 'notice'
  }

  if (errorPattern.test(message)) return 'error'
  if (warnPattern.test(message)) return 'warn'
  if (infoPattern.test(message)) return 'info'
  if (debugPattern.test(message)) return 'debug'
  if (noticePattern.test(message)) return 'notice'

  return 'unknown'
}

export function parseSingleLine(line: string, lineNumber: number): ParsedLogLine {
  const journalctlMatch = line.match(journalctlRegex)
  if (journalctlMatch) {
    const message = journalctlMatch[4]
    return {
      lineNumber,
      raw: line,
      timestamp: journalctlMatch[1],
      hostname: journalctlMatch[2],
      unit: journalctlMatch[3],
      message,
      level: detectLevel(message),
    }
  }

  const launchdMatch = line.match(launchdRegex)
  if (launchdMatch) {
    const message = launchdMatch[4]
    return {
      lineNumber,
      raw: line,
      timestamp: launchdMatch[1],
      hostname: launchdMatch[2],
      unit: launchdMatch[3],
      message,
      level: detectLevel(message),
    }
  }

  return {
    lineNumber,
    raw: line,
    timestamp: '',
    hostname: '',
    unit: '',
    message: line,
    level: detectLevel(line),
  }
}

export function parseLogLines(raw: string): Array<ParsedLogLine> {
  const lines = raw.split('\n').filter((line) => line !== '')
  return lines.map((line, index) => parseSingleLine(line, index + 1))
}
