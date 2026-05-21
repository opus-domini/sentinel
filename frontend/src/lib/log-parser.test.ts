import { describe, expect, it } from 'vitest'
import { detectLevel, parseLogLines, parseSingleLine } from '@/lib/log-parser'

describe('parseSingleLine', () => {
  it('parses journalctl short-iso format', () => {
    const line =
      '2024-01-15T14:32:01+0000 myhost sentinel.service[1234]: Starting service'
    const result = parseSingleLine(line, 1)
    expect(result).toEqual({
      lineNumber: 1,
      raw: line,
      timestamp: '2024-01-15T14:32:01+0000',
      hostname: 'myhost',
      unit: 'sentinel.service[1234]',
      message: 'Starting service',
      level: 'unknown',
    })
  })

  it('parses journalctl with ERROR level in message', () => {
    const line =
      '2024-01-15T14:32:01+0000 myhost app[99]: ERROR connection failed'
    const result = parseSingleLine(line, 5)
    expect(result.level).toBe('error')
    expect(result.message).toBe('ERROR connection failed')
    expect(result.unit).toBe('app[99]')
  })

  it('parses journalctl with WARNING level in message', () => {
    const line =
      '2024-01-15T14:32:01+0000 myhost app[99]: WARNING disk usage high'
    const result = parseSingleLine(line, 3)
    expect(result.level).toBe('warn')
    expect(result.message).toBe('WARNING disk usage high')
  })

  it('parses journalctl unit without PID', () => {
    const line =
      '2024-01-15T14:32:01+0000 myhost sshd: Accepted publickey for user'
    const result = parseSingleLine(line, 1)
    expect(result.unit).toBe('sshd')
    expect(result.message).toBe('Accepted publickey for user')
  })

  it('parses launchd compact format', () => {
    const line =
      '2024-01-15 14:32:01.123456-0700  localhost sentinel: message here'
    const result = parseSingleLine(line, 2)
    expect(result).toEqual({
      lineNumber: 2,
      raw: line,
      timestamp: '2024-01-15 14:32:01.123456-0700',
      hostname: 'localhost',
      unit: 'sentinel',
      message: 'message here',
      level: 'unknown',
    })
  })

  it('returns fallback for unparseable lines', () => {
    const line = 'just some random text'
    const result = parseSingleLine(line, 10)
    expect(result).toEqual({
      lineNumber: 10,
      raw: line,
      timestamp: '',
      hostname: '',
      unit: '',
      message: 'just some random text',
      level: 'unknown',
    })
  })

  it('detects level in unparseable lines', () => {
    const result = parseSingleLine('FATAL: something broke', 1)
    expect(result.level).toBe('error')
    expect(result.message).toBe('FATAL: something broke')
  })

  it('parses lines with level= key-value pattern', () => {
    const line =
      '2024-01-15T14:32:01+0000 myhost app[1]: msg level=error something bad'
    const result = parseSingleLine(line, 1)
    expect(result.level).toBe('error')
  })
})

describe('parseLogLines', () => {
  it('returns empty array for empty input', () => {
    expect(parseLogLines('')).toEqual([])
  })

  it('returns empty array for whitespace-only lines', () => {
    expect(parseLogLines('\n\n\n')).toEqual([])
  })

  it('assigns sequential lineNumber starting at 1', () => {
    const raw = [
      '2024-01-15T14:32:01+0000 myhost app[1]: first line',
      '2024-01-15T14:32:02+0000 myhost app[1]: second line',
      '2024-01-15T14:32:03+0000 myhost app[1]: third line',
    ].join('\n')

    const results = parseLogLines(raw)
    expect(results).toHaveLength(3)
    expect(results[0].lineNumber).toBe(1)
    expect(results[1].lineNumber).toBe(2)
    expect(results[2].lineNumber).toBe(3)
    expect(results[0].message).toBe('first line')
    expect(results[2].message).toBe('third line')
  })

  it('filters out empty lines between entries', () => {
    const raw =
      '2024-01-15T14:32:01+0000 myhost app[1]: line one\n\n2024-01-15T14:32:02+0000 myhost app[1]: line two'
    const results = parseLogLines(raw)
    expect(results).toHaveLength(2)
    expect(results[0].lineNumber).toBe(1)
    expect(results[1].lineNumber).toBe(2)
  })
})

describe('detectLevel', () => {
  const cases: Array<{ input: string; expected: string }> = [
    { input: 'ERROR: something failed', expected: 'error' },
    { input: 'error: lowercase too', expected: 'error' },
    { input: 'FATAL crash detected', expected: 'error' },
    { input: 'CRIT memory exhausted', expected: 'error' },
    { input: 'CRITICAL threshold exceeded', expected: 'error' },
    { input: 'PANIC unrecoverable', expected: 'error' },
    { input: 'WARN disk nearing capacity', expected: 'warn' },
    { input: 'WARNING slow query', expected: 'warn' },
    { input: 'INFO server started', expected: 'info' },
    { input: 'DEBUG inspecting request', expected: 'debug' },
    { input: 'TRACE deep inspection', expected: 'debug' },
    { input: 'NOTICE configuration reloaded', expected: 'notice' },
    { input: 'just a plain message', expected: 'unknown' },
  ]

  for (const { input, expected } of cases) {
    it(`detects "${expected}" from "${input}"`, () => {
      expect(detectLevel(input)).toBe(expected)
    })
  }

  it('detects level from level= key-value pairs', () => {
    expect(detectLevel('msg level=error something')).toBe('error')
    expect(detectLevel('msg level=warn something')).toBe('warn')
    expect(detectLevel('msg level=warning something')).toBe('warn')
    expect(detectLevel('msg level=info something')).toBe('info')
    expect(detectLevel('msg level=debug something')).toBe('debug')
    expect(detectLevel('msg level=notice something')).toBe('notice')
  })

  it('detects level from severity= key-value pairs', () => {
    expect(detectLevel('severity=error request failed')).toBe('error')
    expect(detectLevel('severity=fatal crash')).toBe('error')
    expect(detectLevel('severity=info all good')).toBe('info')
  })

  it('prioritizes key-value pattern over keyword in message body', () => {
    expect(detectLevel('INFO level=error mismatch case')).toBe('error')
  })
})
