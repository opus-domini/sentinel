import { useCallback, useEffect, useRef, useState } from 'react'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { SerializeAddon } from '@xterm/addon-serialize'
import { UnicodeGraphemesAddon } from '@xterm/addon-unicode-graphemes'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import type { RefCallback } from 'react'
import type { ConnectionState } from '@/types'
import type { ReconnectState } from '@/lib/wsReconnect'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { createWebClipboardProvider, writeClipboardText } from '@/lib/clipboardProvider'
import { attachTouchWheelBridge } from '@/lib/touchWheelBridge'
import { THEME_STORAGE_KEY, getTerminalTheme } from '@/lib/terminalThemes'
import { buildWSProtocols } from '@/lib/wsAuth'
import { createReconnect } from '@/lib/wsReconnect'

const MIN_FONT_SIZE = 8
const MAX_FONT_SIZE = 24
const FONT_SIZE_KEY = 'sentinel_font_size'
const TERMINAL_LEFT_GUTTER_PX = 8
const SOCKET_HANDSHAKE_TIMEOUT_MS = 7_000
const TERMINAL_MIN_CONTRAST_RATIO = 4.5
const TERMINAL_WRITE_BATCH_MAX_BYTES = 1_048_576
const TERMINAL_WRITE_FLUSH_FALLBACK_MS = 50
const TERMINAL_WRITE_IN_FLIGHT_TIMEOUT_MS = 5_000
const TERMINAL_WRITE_QUEUE_MAX_BYTES = 16 * 1_048_576
const SELECTION_CLIPBOARD_DEBOUNCE_MS = 120
const TERMINAL_FONT_FAMILY = [
  'JetBrains Mono Variable',
  'JetBrains Mono',
  'Symbols Nerd Font Mono',
  'Symbols Nerd Font',
  'Noto Color Emoji',
  'Apple Color Emoji',
  'Segoe UI Emoji',
  'SF Mono',
  'monospace',
].join(', ')

function applyTerminalChrome(host: HTMLDivElement, themeID: string) {
  const themeBg = getTerminalTheme(themeID).colors.background ?? ''
  host.style.setProperty('background-color', themeBg)
  host.style.setProperty('box-sizing', 'border-box')
  host.style.setProperty('padding-inline-start', `${TERMINAL_LEFT_GUTTER_PX}px`)
}

function loadFontSize(): number {
  const stored = localStorage.getItem(FONT_SIZE_KEY)
  if (stored !== null) {
    const parsed = parseInt(stored, 10)
    if (!isNaN(parsed) && parsed >= MIN_FONT_SIZE && parsed <= MAX_FONT_SIZE) {
      return parsed
    }
  }
  return 13
}

type Disposable = { dispose: () => void }

type SessionRuntime = {
  session: string
  terminal: Terminal
  fitAddon: FitAddon
  encoder: TextEncoder
  socket: WebSocket | null
  generation: number
  manualCloseReason: string | null
  connectionState: ConnectionState
  statusDetail: string
  cols: number
  rows: number
  lastSentCols: number
  lastSentRows: number
  isComposing: boolean
  onDataDispose: Disposable
  onResizeDispose: Disposable
  onSelectionDispose: Disposable
  contextMenuDispose: Disposable
  touchWheelDispose: Disposable
  hostResizeObserver: ResizeObserver | null
  hostResizeRafId: number | null
  reconnect: ReconnectState
  reconnectTimer: number | null
  handshakeTimer: number | null
  writeQueue: Array<Uint8Array>
  writeQueueBytes: number
  writeFlushToken: number
  writeFlushRafId: number | null
  writeFlushTimeoutId: number | null
  writeInFlightGeneration: number | null
  writeInFlightTimeoutId: number | null
  selectionClipboardTimer: number | null
  selectionEndDispose: Disposable
}

type UseTerminalTmuxArgs = {
  openTabs: Array<string>
  activeSession: string
  activeEpoch: number
  sidebarCollapsed: boolean
  onAttachedMobile: () => void
  wsPath?: string
  wsQueryKey?: string
  connectingVerb?: string
  connectedVerb?: string
  allowWheelInAlternateBuffer?: boolean
  suppressBrowserContextMenu?: boolean
}

type UseTerminalTmuxResult = {
  getTerminalHostRef: (session: string) => RefCallback<HTMLDivElement>
  connectionState: ConnectionState
  statusDetail: string
  termCols: number
  termRows: number
  setConnection: (next: ConnectionState, detail: string) => void
  closeCurrentSocket: (reason?: string) => void
  resetTerminal: () => void
  fitTerminal: () => void
  sendKey: (data: string) => void
  flushComposition: () => void
  focusTerminal: () => void
  zoomIn: () => void
  zoomOut: () => void
  reconnectActiveSession: (options?: { force?: boolean }) => void
}

type TerminalRuntimeMetrics = {
  renderer: 'dom'
  writeBatchCount: number
  writeBytes: number
  writeMaxQueueBytes: number
  writeRecoveries: number
  writeBacklogRecoveries: number
  writeStallRecoveries: number
}

function clearRuntimeWriteQueue(runtime: SessionRuntime) {
  if (runtime.writeFlushRafId !== null) {
    window.cancelAnimationFrame(runtime.writeFlushRafId)
    runtime.writeFlushRafId = null
  }
  if (runtime.writeFlushTimeoutId !== null) {
    window.clearTimeout(runtime.writeFlushTimeoutId)
    runtime.writeFlushTimeoutId = null
  }
  if (runtime.writeInFlightTimeoutId !== null) {
    window.clearTimeout(runtime.writeInFlightTimeoutId)
    runtime.writeInFlightTimeoutId = null
  }
  runtime.writeFlushToken += 1
  runtime.writeQueue = []
  runtime.writeQueueBytes = 0
  runtime.writeInFlightGeneration = null
}

function refreshRuntimeRenderer(runtime: SessionRuntime): boolean {
  if (!runtime.terminal.element) {
    return false
  }
  runtime.terminal.refresh(0, Math.max(0, runtime.terminal.rows - 1))
  return true
}

function takeRuntimeWriteBatch(runtime: SessionRuntime, maxBytes: number): Uint8Array | null {
  if (runtime.writeQueueBytes <= 0 || runtime.writeQueue.length === 0) {
    return null
  }

  let batchBytes = 0
  let batchCount = 0
  for (const chunk of runtime.writeQueue) {
    if (batchBytes > 0 && batchBytes + chunk.byteLength > maxBytes) {
      break
    }
    batchBytes += chunk.byteLength
    batchCount += 1
    if (batchBytes >= maxBytes) {
      break
    }
  }

  const chunks = runtime.writeQueue.splice(0, batchCount)
  runtime.writeQueueBytes -= batchBytes

  if (chunks.length === 1) {
    return chunks[0]
  }

  const batch = new Uint8Array(batchBytes)
  let offset = 0
  for (const chunk of chunks) {
    batch.set(chunk, offset)
    offset += chunk.byteLength
  }
  return batch
}

function isSocketOpenOrConnecting(socket: WebSocket | null): boolean {
  if (socket === null) {
    return false
  }
  return socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING
}

export function useTerminalTmux({
  openTabs,
  activeSession,
  activeEpoch,
  sidebarCollapsed,
  onAttachedMobile,
  wsPath = '/ws/tmux',
  wsQueryKey = 'session',
  connectingVerb = 'opening',
  connectedVerb = 'attached',
  allowWheelInAlternateBuffer = false,
  suppressBrowserContextMenu = false,
}: UseTerminalTmuxArgs): UseTerminalTmuxResult {
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected')
  const [statusDetail, setStatusDetail] = useState('ready')
  const [termCols, setTermCols] = useState(0)
  const [termRows, setTermRows] = useState(0)
  const [fontSize, setFontSize] = useState(loadFontSize)
  const [themeId, setThemeId] = useState(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'sentinel',
  )

  const isMobile = useIsMobileLayout()
  const isMobileRef = useRef(isMobile)
  isMobileRef.current = isMobile

  const isMountedRef = useRef(true)
  const activeSessionRef = useRef(activeSession)
  const runtimesRef = useRef(new Map<string, SessionRuntime>())
  const hostsRef = useRef(new Map<string, HTMLDivElement>())
  const hostCallbacksRef = useRef(new Map<string, RefCallback<HTMLDivElement>>())
  const terminalMetricsRef = useRef<TerminalRuntimeMetrics>({
    renderer: 'dom',
    writeBatchCount: 0,
    writeBytes: 0,
    writeMaxQueueBytes: 0,
    writeRecoveries: 0,
    writeBacklogRecoveries: 0,
    writeStallRecoveries: 0,
  })

  const setRuntimeStatus = useCallback(
    (runtime: SessionRuntime, next: ConnectionState, detail: string) => {
      runtime.connectionState = next
      runtime.statusDetail = detail
      if (!isMountedRef.current || activeSessionRef.current !== runtime.session) {
        return
      }
      setConnectionState(next)
      setStatusDetail(detail)
    },
    [],
  )

  const clearReconnectTimer = useCallback((runtime: SessionRuntime) => {
    if (runtime.reconnectTimer !== null) {
      window.clearTimeout(runtime.reconnectTimer)
      runtime.reconnectTimer = null
    }
  }, [])

  const clearHandshakeTimer = useCallback((runtime: SessionRuntime) => {
    if (runtime.handshakeTimer !== null) {
      window.clearTimeout(runtime.handshakeTimer)
      runtime.handshakeTimer = null
    }
  }, [])

  const publishActiveRuntimeState = useCallback(() => {
    if (!isMountedRef.current) {
      return
    }

    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') {
      setConnectionState('disconnected')
      setStatusDetail('no session attached')
      setTermCols(0)
      setTermRows(0)
      return
    }

    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) {
      setConnectionState('disconnected')
      setStatusDetail('disconnected')
      setTermCols(0)
      setTermRows(0)
      return
    }

    setConnectionState(runtime.connectionState)
    setStatusDetail(runtime.statusDetail)
    setTermCols(runtime.cols)
    setTermRows(runtime.rows)
  }, [])

  const sendResize = useCallback((runtime: SessionRuntime, cols: number, rows: number) => {
    if (cols <= 0 || rows <= 0) {
      return
    }
    const socket = runtime.socket
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return
    }
    if (cols === runtime.lastSentCols && rows === runtime.lastSentRows) {
      return
    }
    socket.send(JSON.stringify({ type: 'resize', cols, rows }))
    runtime.lastSentCols = cols
    runtime.lastSentRows = rows
  }, [])

  const cleanupHostResizeObserver = useCallback((runtime: SessionRuntime) => {
    runtime.hostResizeObserver?.disconnect()
    runtime.hostResizeObserver = null
    if (runtime.hostResizeRafId !== null) {
      window.cancelAnimationFrame(runtime.hostResizeRafId)
      runtime.hostResizeRafId = null
    }
  }, [])

  const refreshRuntime = useCallback((runtime: SessionRuntime) => {
    return refreshRuntimeRenderer(runtime)
  }, [])

  const fitRuntime = useCallback(
    (runtime: SessionRuntime, options?: { forceRefresh?: boolean }) => {
      if (!runtime.terminal.element) {
        return
      }
      const previousCols = runtime.cols
      const previousRows = runtime.rows
      runtime.fitAddon.fit()
      runtime.cols = runtime.terminal.cols
      runtime.rows = runtime.terminal.rows
      const sizeChanged = runtime.cols !== previousCols || runtime.rows !== previousRows
      sendResize(runtime, runtime.cols, runtime.rows)
      if (sizeChanged || options?.forceRefresh === true) {
        refreshRuntimeRenderer(runtime)
      }

      if (!isMountedRef.current || activeSessionRef.current !== runtime.session) {
        return
      }
      setTermCols(runtime.cols)
      setTermRows(runtime.rows)
    },
    [sendResize],
  )

  const refreshActiveRuntime = useCallback(() => {
    const runtime = runtimesRef.current.get(activeSessionRef.current.trim())
    if (runtime) {
      refreshRuntime(runtime)
    }
  }, [refreshRuntime])

  const observeHostResize = useCallback(
    (runtime: SessionRuntime, host: HTMLDivElement) => {
      cleanupHostResizeObserver(runtime)
      if (typeof ResizeObserver === 'undefined') {
        return
      }

      const queueFit = () => {
        if (runtime.hostResizeRafId !== null) {
          window.cancelAnimationFrame(runtime.hostResizeRafId)
        }
        runtime.hostResizeRafId = window.requestAnimationFrame(() => {
          runtime.hostResizeRafId = null
          if (
            activeSessionRef.current !== runtime.session ||
            runtimesRef.current.get(runtime.session) !== runtime
          ) {
            return
          }
          fitRuntime(runtime)
        })
      }

      const observer = new ResizeObserver(() => {
        queueFit()
      })
      observer.observe(host)
      runtime.hostResizeObserver = observer
      queueFit()
    },
    [cleanupHostResizeObserver, fitRuntime],
  )

  const openRuntimeInHost = useCallback(
    (runtime: SessionRuntime, host: HTMLDivElement) => {
      applyTerminalChrome(host, themeId)
      observeHostResize(runtime, host)
      if (runtime.terminal.element) {
        runtime.terminal.element.style.backgroundColor =
          getTerminalTheme(themeId).colors.background ?? ''
        runtime.touchWheelDispose.dispose()
        runtime.touchWheelDispose = { dispose: () => undefined }
        if (isMobileRef.current && allowWheelInAlternateBuffer) {
          const screen = runtime.terminal.element.querySelector<HTMLElement>('.xterm-screen')
          runtime.touchWheelDispose = attachTouchWheelBridge({
            host,
            dispatchTarget: screen ?? runtime.terminal.element,
          })
        }
        refreshRuntime(runtime)
        return
      }

      const openTerminal = () => {
        if (runtimesRef.current.get(runtime.session) !== runtime) {
          return
        }
        if (!host.isConnected || runtime.terminal.element) {
          return
        }
        runtime.terminal.open(host)

        // Disable IME prediction/autocorrect on the hidden textarea so
        // mobile keyboards send characters one-by-one instead of buffering
        // whole words via composition.
        const ta = runtime.terminal.textarea
        if (ta) {
          ta.setAttribute(
            'name',
            `terminal-input-${runtime.session.replace(/[^a-zA-Z0-9_-]+/g, '-')}`,
          )
          ta.setAttribute('autocorrect', 'off')
          ta.setAttribute('autocomplete', 'off')
          ta.setAttribute('autocapitalize', 'off')
          ta.setAttribute('spellcheck', 'false')
          ta.setAttribute('writingsuggestions', 'false')

          // Track composition lifecycle and clear the textarea after each
          // cycle.  xterm.js never clears the textarea, so text accumulates
          // across compositions.  After cursor movement via ExtraKeys (which
          // bypass the textarea), the stale content causes xterm's substring-
          // based diff to re-send previously entered characters.
          //
          // Clearing happens in a setTimeout(fn, 0) queued AFTER xterm's own
          // setTimeout in _finalizeComposition(true), so xterm reads the
          // composed text first, then we reset the textarea for the next cycle.
          ta.addEventListener('compositionstart', () => {
            runtime.isComposing = true
          })
          ta.addEventListener('compositionend', () => {
            runtime.isComposing = false
            setTimeout(() => {
              if (!runtime.isComposing && ta.value !== '') {
                ta.value = ''
              }
            }, 0)
          })
          ta.addEventListener('focus', () => {
            if (!isMobileRef.current) return
            requestAnimationFrame(() => {
              fitRuntime(runtime)
              runtime.terminal.scrollToBottom()
            })
          })
        }

        if (suppressBrowserContextMenu) {
          const handleContextMenu = (event: Event) => {
            event.preventDefault()
          }
          host.addEventListener('contextmenu', handleContextMenu)
          runtime.contextMenuDispose = {
            dispose: () => {
              host.removeEventListener('contextmenu', handleContextMenu)
            },
          }
        }

        runtime.touchWheelDispose.dispose()
        runtime.touchWheelDispose = { dispose: () => undefined }
        if (isMobileRef.current && allowWheelInAlternateBuffer) {
          const dispatchTarget = host.querySelector<HTMLElement>('.xterm-screen')
          runtime.touchWheelDispose = attachTouchWheelBridge({
            host,
            dispatchTarget: dispatchTarget ?? host,
          })
        }

        // FitAddon floors column/row counts, so a residual strip can appear.
        // Keep the xterm root in sync with theme background to hide it.
        applyTerminalChrome(host, themeId)
        const terminalElement = runtime.terminal.element as HTMLElement | undefined
        if (terminalElement) {
          terminalElement.style.backgroundColor = getTerminalTheme(themeId).colors.background ?? ''
        }
        refreshRuntime(runtime)

        fitRuntime(runtime)
      }

      void document.fonts.ready.then(openTerminal).catch(() => openTerminal())
    },
    [
      fitRuntime,
      observeHostResize,
      suppressBrowserContextMenu,
      themeId,
      allowWheelInAlternateBuffer,
      refreshRuntime,
    ],
  )

  const isSocketCurrent = useCallback(
    (runtime: SessionRuntime, generation: number, socket: WebSocket): boolean => {
      return (
        runtimesRef.current.get(runtime.session) === runtime &&
        runtime.generation === generation &&
        runtime.socket === socket
      )
    },
    [],
  )

  const isRuntimeCurrent = useCallback((runtime: SessionRuntime, generation: number): boolean => {
    return runtimesRef.current.get(runtime.session) === runtime && runtime.generation === generation
  }, [])

  const recoverRuntimeWritePipeline = useCallback(
    (runtime: SessionRuntime, detail: string, reason: 'backlog' | 'stall') => {
      terminalMetricsRef.current.writeRecoveries += 1
      if (reason === 'backlog') {
        terminalMetricsRef.current.writeBacklogRecoveries += 1
      } else {
        terminalMetricsRef.current.writeStallRecoveries += 1
      }
      clearRuntimeWriteQueue(runtime)
      runtime.terminal.reset()
      setRuntimeStatus(runtime, 'connecting', detail)
      if (runtime.socket && isSocketOpenOrConnecting(runtime.socket)) {
        runtime.manualCloseReason = null
        runtime.socket.close()
      }
    },
    [setRuntimeStatus],
  )

  const enqueueRuntimeWrite = useCallback(
    (runtime: SessionRuntime, generation: number, chunk: Uint8Array) => {
      if (chunk.byteLength === 0 || !isRuntimeCurrent(runtime, generation)) {
        return
      }

      runtime.writeQueue.push(chunk)
      runtime.writeQueueBytes += chunk.byteLength
      terminalMetricsRef.current.writeMaxQueueBytes = Math.max(
        terminalMetricsRef.current.writeMaxQueueBytes,
        runtime.writeQueueBytes,
      )
      if (runtime.writeQueueBytes > TERMINAL_WRITE_QUEUE_MAX_BYTES) {
        recoverRuntimeWritePipeline(runtime, 'terminal output backlog; reconnecting', 'backlog')
        return
      }
      if (
        runtime.writeFlushRafId !== null ||
        runtime.writeFlushTimeoutId !== null ||
        runtime.writeInFlightGeneration !== null
      ) {
        return
      }

      const flush = (flushToken: number) => {
        if (runtime.writeFlushToken !== flushToken || !isRuntimeCurrent(runtime, generation)) {
          return
        }
        runtime.writeFlushToken += 1
        if (runtime.writeFlushRafId !== null) {
          window.cancelAnimationFrame(runtime.writeFlushRafId)
          runtime.writeFlushRafId = null
        }
        if (runtime.writeFlushTimeoutId !== null) {
          window.clearTimeout(runtime.writeFlushTimeoutId)
          runtime.writeFlushTimeoutId = null
        }

        const payload = takeRuntimeWriteBatch(runtime, TERMINAL_WRITE_BATCH_MAX_BYTES)
        if (payload !== null) {
          runtime.writeInFlightGeneration = generation
          terminalMetricsRef.current.writeBatchCount += 1
          terminalMetricsRef.current.writeBytes += payload.byteLength
          runtime.writeInFlightTimeoutId = window.setTimeout(() => {
            if (
              runtime.writeInFlightGeneration !== generation ||
              !isRuntimeCurrent(runtime, generation)
            ) {
              return
            }
            recoverRuntimeWritePipeline(runtime, 'terminal renderer stalled; reconnecting', 'stall')
          }, TERMINAL_WRITE_IN_FLIGHT_TIMEOUT_MS)
          runtime.terminal.write(payload, () => {
            if (runtime.writeInFlightGeneration !== generation) {
              return
            }
            if (runtime.writeInFlightTimeoutId !== null) {
              window.clearTimeout(runtime.writeInFlightTimeoutId)
              runtime.writeInFlightTimeoutId = null
            }
            runtime.writeInFlightGeneration = null
            if (!isRuntimeCurrent(runtime, generation)) {
              return
            }
            if (runtime.writeQueue.length > 0) {
              scheduleFlush()
            }
          })
        }
      }

      const scheduleFlush = () => {
        const flushToken = runtime.writeFlushToken + 1
        runtime.writeFlushToken = flushToken
        runtime.writeFlushRafId = window.requestAnimationFrame(() => {
          flush(flushToken)
        })
        runtime.writeFlushTimeoutId = window.setTimeout(() => {
          flush(flushToken)
        }, TERMINAL_WRITE_FLUSH_FALLBACK_MS)
      }

      scheduleFlush()
    },
    [isRuntimeCurrent, recoverRuntimeWritePipeline],
  )

  const connectRuntime = useCallback(
    (runtime: SessionRuntime, options?: { resetTerminal?: boolean; force?: boolean }) => {
      if (!options?.force && isSocketOpenOrConnecting(runtime.socket)) {
        return
      }

      clearReconnectTimer(runtime)
      clearHandshakeTimer(runtime)
      runtime.generation += 1
      const generation = runtime.generation
      clearRuntimeWriteQueue(runtime)

      const previousSocket = runtime.socket
      if (previousSocket) {
        if (!runtime.manualCloseReason) {
          runtime.manualCloseReason = 'reconnecting'
        }
        runtime.socket = null
        previousSocket.close()
      }

      if (options?.resetTerminal !== false) {
        runtime.terminal.reset()
      }
      setRuntimeStatus(runtime, 'connecting', `${connectingVerb} ${runtime.session}`)

      const wsURL = new URL(wsPath, window.location.origin)
      wsURL.searchParams.set(wsQueryKey, runtime.session)
      if (runtime.cols > 0 && runtime.rows > 0) {
        wsURL.searchParams.set('cols', String(runtime.cols))
        wsURL.searchParams.set('rows', String(runtime.rows))
      }
      const socket = new WebSocket(wsURL.toString().replace(/^http/, 'ws'), buildWSProtocols())
      socket.binaryType = 'arraybuffer'
      runtime.socket = socket
      runtime.lastSentCols = 0
      runtime.lastSentRows = 0
      runtime.handshakeTimer = window.setTimeout(() => {
        runtime.handshakeTimer = null
        if (!isSocketCurrent(runtime, generation, socket)) {
          return
        }
        if (socket.readyState !== WebSocket.CONNECTING) {
          return
        }
        socket.close()
      }, SOCKET_HANDSHAKE_TIMEOUT_MS)

      socket.onopen = () => {
        clearHandshakeTimer(runtime)
        if (!isSocketCurrent(runtime, generation, socket)) {
          return
        }
        runtime.manualCloseReason = null
        runtime.reconnect.reset()
        setRuntimeStatus(runtime, 'connected', `${connectedVerb} ${runtime.session}`)
        fitRuntime(runtime)

        if (activeSessionRef.current !== runtime.session) {
          return
        }
        if (isMobileRef.current) {
          onAttachedMobile()
        } else {
          runtime.terminal.focus()
        }
      }

      socket.onmessage = (event) => {
        if (!isSocketCurrent(runtime, generation, socket)) {
          return
        }

        if (typeof event.data === 'string') {
          try {
            const message = JSON.parse(event.data) as {
              type?: string
              message?: string
            }
            if (message.type === 'error') {
              setRuntimeStatus(runtime, 'error', message.message ?? 'terminal error')
            }
          } catch {
            // ignore invalid control frame
          }
          return
        }

        if (event.data instanceof ArrayBuffer) {
          enqueueRuntimeWrite(runtime, generation, new Uint8Array(event.data))
          return
        }

        if (event.data instanceof Blob) {
          void event.data.arrayBuffer().then((buffer) => {
            if (!isSocketCurrent(runtime, generation, socket)) {
              return
            }
            enqueueRuntimeWrite(runtime, generation, new Uint8Array(buffer))
          })
        }
      }

      socket.onerror = () => {
        if (!isSocketCurrent(runtime, generation, socket)) {
          return
        }
        setRuntimeStatus(runtime, 'error', 'websocket error')
      }

      socket.onclose = () => {
        clearHandshakeTimer(runtime)
        if (!isRuntimeCurrent(runtime, generation)) {
          return
        }
        if (runtime.socket === socket) {
          runtime.socket = null
        }
        const wasManual = runtime.manualCloseReason !== null
        const reason = runtime.manualCloseReason ?? 'connection closed'
        runtime.manualCloseReason = null

        if (wasManual) {
          setRuntimeStatus(runtime, 'disconnected', reason)
          return
        }

        // Unexpected close — schedule auto-reconnect with backoff
        const delay = runtime.reconnect.next()
        const delaySec = Math.ceil(delay / 1000)
        setRuntimeStatus(runtime, 'connecting', `reconnecting in ${delaySec}s`)
        runtime.reconnectTimer = window.setTimeout(() => {
          runtime.reconnectTimer = null
          if (isRuntimeCurrent(runtime, generation)) {
            connectRuntime(runtime, { resetTerminal: false })
          }
        }, delay)
      }
    },
    [
      clearHandshakeTimer,
      clearReconnectTimer,
      connectedVerb,
      connectingVerb,
      enqueueRuntimeWrite,
      fitRuntime,
      isRuntimeCurrent,
      isSocketCurrent,
      onAttachedMobile,
      setRuntimeStatus,
      wsPath,
      wsQueryKey,
    ],
  )

  const closeRuntimeSocket = useCallback(
    (runtime: SessionRuntime, reason?: string) => {
      clearReconnectTimer(runtime)
      clearHandshakeTimer(runtime)

      if (reason && reason !== '') {
        runtime.manualCloseReason = reason
      }

      const socket = runtime.socket
      if (!socket) {
        if (reason && reason !== '') {
          setRuntimeStatus(runtime, 'disconnected', reason)
        }
        return
      }

      runtime.socket = null
      socket.close()
    },
    [clearHandshakeTimer, clearReconnectTimer, setRuntimeStatus],
  )

  const createRuntime = useCallback(
    (session: string): SessionRuntime => {
      const terminal = new Terminal({
        // Required to switch unicode.activeVersion to the graphemes
        // provider below — xterm gates the unicode API behind this flag.
        allowProposedApi: true,
        customGlyphs: true,
        cursorBlink: true,
        fontFamily: TERMINAL_FONT_FAMILY,
        fontSize,
        lineHeight: 1,
        minimumContrastRatio: TERMINAL_MIN_CONTRAST_RATIO,
        rescaleOverlappingGlyphs: true,
        scrollback: 5000,
        smoothScrollDuration: 0,
        rightClickSelectsWord: false,
        theme: getTerminalTheme(themeId).colors,
      })

      // In alternate screen buffers (tmux/vim), xterm.js converts wheel
      // gestures into arrow key sequences. Keep this blocked by default,
      // but allow callers (tmux route) to opt-in so tmux can handle wheel.
      if (!allowWheelInAlternateBuffer) {
        terminal.attachCustomWheelEventHandler(() => {
          return terminal.buffer.active.type !== 'alternate'
        })
      }

      const fitAddon = new FitAddon()
      const clipboardAddon = new ClipboardAddon(undefined, createWebClipboardProvider())
      const searchAddon = new SearchAddon({ highlightLimit: 2_000 })
      const serializeAddon = new SerializeAddon()
      const webLinksAddon = new WebLinksAddon((event, uri) => {
        event.preventDefault()
        window.open(uri, '_blank', 'noopener,noreferrer')
      })
      // Align xterm's column-width table with modern tmux (Unicode 15 +
      // grapheme clusters). Without this, emoji, flags, ZWJ sequences and
      // CJK render as 1 column in xterm while tmux advances the cursor by
      // 2 — the buffer then drifts out of sync and the screen garbles.
      const unicodeGraphemesAddon = new UnicodeGraphemesAddon()

      terminal.loadAddon(fitAddon)
      terminal.loadAddon(clipboardAddon)
      terminal.loadAddon(searchAddon)
      terminal.loadAddon(serializeAddon)
      terminal.loadAddon(webLinksAddon)
      terminal.loadAddon(unicodeGraphemesAddon)
      terminal.unicode.activeVersion = '15-graphemes'

      const runtime: SessionRuntime = {
        session,
        terminal,
        fitAddon,
        encoder: new TextEncoder(),
        socket: null,
        generation: 0,
        manualCloseReason: null,
        connectionState: 'disconnected',
        statusDetail: 'ready',
        cols: 0,
        rows: 0,
        lastSentCols: 0,
        lastSentRows: 0,
        isComposing: false,
        onDataDispose: { dispose: () => undefined },
        onResizeDispose: { dispose: () => undefined },
        onSelectionDispose: { dispose: () => undefined },
        contextMenuDispose: { dispose: () => undefined },
        touchWheelDispose: { dispose: () => undefined },
        hostResizeObserver: null,
        hostResizeRafId: null,
        reconnect: createReconnect(),
        reconnectTimer: null,
        handshakeTimer: null,
        writeQueue: [],
        writeQueueBytes: 0,
        writeFlushToken: 0,
        writeFlushRafId: null,
        writeFlushTimeoutId: null,
        writeInFlightGeneration: null,
        writeInFlightTimeoutId: null,
        selectionClipboardTimer: null,
        selectionEndDispose: { dispose: () => undefined },
      }

      runtime.onDataDispose = terminal.onData((data) => {
        const socket = runtime.socket
        if (!socket || socket.readyState !== WebSocket.OPEN) {
          return
        }
        socket.send(runtime.encoder.encode(data))
      })

      runtime.onResizeDispose = terminal.onResize(({ cols, rows }) => {
        runtime.cols = cols
        runtime.rows = rows
        sendResize(runtime, cols, rows)

        if (!isMountedRef.current || activeSessionRef.current !== runtime.session) {
          return
        }
        setTermCols(cols)
        setTermRows(rows)
      })

      // Copy xterm.js native selection (Shift+drag) to the system clipboard.
      // Async Clipboard can be debounced; the textarea fallback must run during
      // a real user event, so non-secure contexts flush on pointer/key end.
      const hasAsyncClipboard = () => (navigator.clipboard as Clipboard | undefined) != null
      const clearSelectionClipboardTimer = () => {
        if (runtime.selectionClipboardTimer === null) return
        window.clearTimeout(runtime.selectionClipboardTimer)
        runtime.selectionClipboardTimer = null
      }
      const writeCurrentSelection = () => {
        clearSelectionClipboardTimer()
        const text = terminal.getSelection()
        if (text) {
          writeClipboardText(text)
        }
      }
      runtime.onSelectionDispose = terminal.onSelectionChange(() => {
        if (!hasAsyncClipboard()) return
        clearSelectionClipboardTimer()
        runtime.selectionClipboardTimer = window.setTimeout(() => {
          runtime.selectionClipboardTimer = null
          writeCurrentSelection()
        }, SELECTION_CLIPBOARD_DEBOUNCE_MS)
      })
      const writeSelectionOnUserEnd = () => {
        if (hasAsyncClipboard()) return
        if (activeSessionRef.current !== runtime.session) return
        writeCurrentSelection()
      }
      document.addEventListener('pointerup', writeSelectionOnUserEnd, true)
      document.addEventListener('mouseup', writeSelectionOnUserEnd, true)
      document.addEventListener('touchend', writeSelectionOnUserEnd, true)
      document.addEventListener('keyup', writeSelectionOnUserEnd, true)
      runtime.selectionEndDispose = {
        dispose: () => {
          document.removeEventListener('pointerup', writeSelectionOnUserEnd, true)
          document.removeEventListener('mouseup', writeSelectionOnUserEnd, true)
          document.removeEventListener('touchend', writeSelectionOnUserEnd, true)
          document.removeEventListener('keyup', writeSelectionOnUserEnd, true)
        },
      }

      runtimesRef.current.set(session, runtime)

      const host = hostsRef.current.get(session)
      if (host) {
        openRuntimeInHost(runtime, host)
      }

      // Don't connect immediately — the active-session effect handles
      // connecting the visible tab, and background tabs connect lazily
      // when the user switches to them.  This prevents N simultaneous
      // WebSocket connections on page load from exhausting Chrome's
      // per-origin socket pool (max 6 for HTTP/1.1).
      return runtime
    },
    [allowWheelInAlternateBuffer, fontSize, openRuntimeInHost, sendResize, themeId],
  )

  const disposeRuntime = useCallback(
    (runtime: SessionRuntime, reason: string) => {
      clearReconnectTimer(runtime)
      clearHandshakeTimer(runtime)
      runtime.generation += 1
      closeRuntimeSocket(runtime, reason)
      cleanupHostResizeObserver(runtime)
      runtime.onDataDispose.dispose()
      runtime.onResizeDispose.dispose()
      runtime.onSelectionDispose.dispose()
      runtime.selectionEndDispose.dispose()
      if (runtime.selectionClipboardTimer !== null) {
        window.clearTimeout(runtime.selectionClipboardTimer)
        runtime.selectionClipboardTimer = null
      }
      runtime.contextMenuDispose.dispose()
      runtime.touchWheelDispose.dispose()
      clearRuntimeWriteQueue(runtime)
      runtime.terminal.dispose()
      runtimesRef.current.delete(runtime.session)
      hostsRef.current.delete(runtime.session)
      hostCallbacksRef.current.delete(runtime.session)
    },
    [clearHandshakeTimer, clearReconnectTimer, cleanupHostResizeObserver, closeRuntimeSocket],
  )

  const getTerminalHostRef = useCallback(
    (session: string): RefCallback<HTMLDivElement> => {
      const existing = hostCallbacksRef.current.get(session)
      if (existing) {
        return existing
      }

      const callback: RefCallback<HTMLDivElement> = (node) => {
        if (node) {
          hostsRef.current.set(session, node)
        } else {
          hostsRef.current.delete(session)
        }

        const runtime = runtimesRef.current.get(session)
        if (runtime && node) {
          openRuntimeInHost(runtime, node)
        } else if (runtime) {
          cleanupHostResizeObserver(runtime)
          runtime.touchWheelDispose.dispose()
          runtime.touchWheelDispose = { dispose: () => undefined }
        }
      }

      hostCallbacksRef.current.set(session, callback)
      return callback
    },
    [cleanupHostResizeObserver, openRuntimeInHost],
  )

  const setConnection = useCallback((next: ConnectionState, detail: string) => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName !== '') {
      const runtime = runtimesRef.current.get(sessionName)
      if (runtime) {
        // Never regress a connected runtime to 'connecting'. The WebSocket
        // state machine (socket.onopen) is the authority once connected.
        // External callers can target the wrong runtime when activeSessionRef
        // is stale (e.g. during session creation before React re-renders).
        if (runtime.connectionState === 'connected' && next === 'connecting') {
          return
        }
        runtime.connectionState = next
        runtime.statusDetail = detail
      }
    }

    if (!isMountedRef.current) {
      return
    }
    setConnectionState(next)
    setStatusDetail(detail)
  }, [])

  const closeCurrentSocket = useCallback(
    (reason?: string) => {
      const sessionName = activeSessionRef.current.trim()
      if (sessionName === '') {
        return
      }
      const runtime = runtimesRef.current.get(sessionName)
      if (!runtime) {
        return
      }
      closeRuntimeSocket(runtime, reason)
    },
    [closeRuntimeSocket],
  )

  const resetTerminal = useCallback(() => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') {
      return
    }
    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) {
      return
    }
    clearRuntimeWriteQueue(runtime)
    runtime.terminal.reset()
  }, [])

  const fitTerminal = useCallback(() => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') {
      return
    }
    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) {
      return
    }
    fitRuntime(runtime)
  }, [fitRuntime])

  const sendKey = useCallback((data: string) => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') {
      return
    }
    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) {
      return
    }
    const socket = runtime.socket
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return
    }
    socket.send(runtime.encoder.encode(data))
  }, [])

  const flushComposition = useCallback(() => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') return
    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) return
    const ta = runtime.terminal.textarea
    if (!ta) return

    if (runtime.isComposing) {
      // Dispatch a synthetic compositionend so xterm's CompositionHelper
      // calls _finalizeComposition(true), reads the pending text and sends
      // it to the PTY.  Our compositionend listener then clears the textarea.
      ta.dispatchEvent(new CompositionEvent('compositionend'))
    } else if (ta.value !== '') {
      // No active composition — clear stale text that accumulated from
      // previous composition cycles so the next input starts fresh.
      ta.value = ''
    }
  }, [])

  const applyFontSize = useCallback(
    (size: number) => {
      setFontSize(size)
      localStorage.setItem(FONT_SIZE_KEY, String(size))
      for (const runtime of runtimesRef.current.values()) {
        runtime.terminal.options.fontSize = size
        if (runtime.session === activeSessionRef.current.trim() && runtime.terminal.element) {
          fitRuntime(runtime, { forceRefresh: true })
        }
      }
    },
    [fitRuntime],
  )

  const zoomIn = useCallback(() => {
    setFontSize((prev) => {
      const next = Math.min(prev + 1, MAX_FONT_SIZE)
      if (next !== prev) applyFontSize(next)
      return next
    })
  }, [applyFontSize])

  const zoomOut = useCallback(() => {
    setFontSize((prev) => {
      const next = Math.max(prev - 1, MIN_FONT_SIZE)
      if (next !== prev) applyFontSize(next)
      return next
    })
  }, [applyFontSize])

  const applyTheme = useCallback(
    (id: string) => {
      setThemeId(id)
      const colors = getTerminalTheme(id).colors
      for (const runtime of runtimesRef.current.values()) {
        runtime.terminal.options.theme = colors
        const host = hostsRef.current.get(runtime.session)
        if (host) {
          applyTerminalChrome(host, id)
        }
        if (runtime.terminal.element) {
          runtime.terminal.element.style.backgroundColor = colors.background ?? ''
          refreshRuntime(runtime)
        }
      }
    },
    [refreshRuntime],
  )

  useEffect(() => {
    ;(
      window as typeof window & { __SENTINEL_TERMINAL_METRICS?: unknown }
    ).__SENTINEL_TERMINAL_METRICS = terminalMetricsRef.current
    return () => {
      ;(
        window as typeof window & { __SENTINEL_TERMINAL_METRICS?: unknown }
      ).__SENTINEL_TERMINAL_METRICS = undefined
    }
  }, [])

  useEffect(() => {
    const handler = (event: Event) => {
      const id = (event as CustomEvent<string>).detail
      if (typeof id === 'string') {
        applyTheme(id)
      }
    }
    window.addEventListener('sentinel-theme-change', handler)
    return () => {
      window.removeEventListener('sentinel-theme-change', handler)
    }
  }, [applyTheme])

  const focusTerminal = useCallback(() => {
    const sessionName = activeSessionRef.current.trim()
    if (sessionName === '') {
      return
    }
    const runtime = runtimesRef.current.get(sessionName)
    if (!runtime) {
      return
    }
    runtime.terminal.focus()
  }, [])

  const prevActiveSessionRef = useRef('')

  useEffect(() => {
    const activeName = activeSession.trim()
    const prevActive = prevActiveSessionRef.current
    activeSessionRef.current = activeSession
    prevActiveSessionRef.current = activeName
    publishActiveRuntimeState()
    fitTerminal()

    // Disconnect the PREVIOUS session's socket when switching tabs,
    // freeing a Chrome socket pool slot for the new session.
    if (prevActive !== '' && prevActive !== activeName) {
      const prevRuntime = runtimesRef.current.get(prevActive)
      if (prevRuntime && isSocketOpenOrConnecting(prevRuntime.socket)) {
        closeRuntimeSocket(prevRuntime, 'background')
      }
    }

    const runtime = runtimesRef.current.get(activeName)
    if (!runtime) {
      return
    }

    if (!isSocketOpenOrConnecting(runtime.socket)) {
      connectRuntime(runtime, { resetTerminal: false })
    }

    refreshRuntime(runtime)

    if (!isMobileRef.current) {
      runtime.terminal.focus()
    }
  }, [
    activeEpoch,
    activeSession,
    closeRuntimeSocket,
    connectRuntime,
    fitTerminal,
    publishActiveRuntimeState,
    refreshRuntime,
  ])

  useEffect(() => {
    const allowedSessions = new Set(openTabs.filter((session) => session.trim() !== ''))

    const activeName = activeSessionRef.current.trim()
    for (const session of allowedSessions) {
      if (!runtimesRef.current.has(session)) {
        const rt = createRuntime(session)
        // Connect newly created runtimes for the active session so the
        // terminal attaches on initial page load.  Subsequent session
        // switches are handled by the activeSession effect.
        if (session === activeName && !isSocketOpenOrConnecting(rt.socket)) {
          connectRuntime(rt, { resetTerminal: false })
        }
      }
    }

    for (const runtime of [...runtimesRef.current.values()]) {
      if (!allowedSessions.has(runtime.session)) {
        disposeRuntime(runtime, 'tab closed')
      }
    }

    publishActiveRuntimeState()
  }, [connectRuntime, createRuntime, disposeRuntime, openTabs, publishActiveRuntimeState])

  useEffect(() => {
    for (const runtime of runtimesRef.current.values()) {
      runtime.touchWheelDispose.dispose()
      runtime.touchWheelDispose = { dispose: () => undefined }
      if (!isMobile || !allowWheelInAlternateBuffer) {
        continue
      }
      const host = hostsRef.current.get(runtime.session)
      const terminalElement = runtime.terminal.element
      if (!host || !terminalElement) {
        continue
      }
      const screen = terminalElement.querySelector<HTMLElement>('.xterm-screen')
      runtime.touchWheelDispose = attachTouchWheelBridge({
        host,
        dispatchTarget: screen ?? terminalElement,
      })
    }
  }, [allowWheelInAlternateBuffer, isMobile])

  useEffect(() => {
    const onWindowResize = () => {
      fitTerminal()
    }

    window.addEventListener('resize', onWindowResize)
    return () => {
      window.removeEventListener('resize', onWindowResize)
    }
  }, [fitTerminal])

  useEffect(() => {
    if (!window.visualViewport) {
      return
    }
    const vv = window.visualViewport
    const onViewportChange = () => {
      if (!isMobileRef.current) return
      const runtime = runtimesRef.current.get(activeSessionRef.current.trim())
      if (!runtime || !runtime.terminal.element) return
      fitRuntime(runtime)
      if (document.documentElement.classList.contains('keyboard-visible')) {
        runtime.terminal.scrollToBottom()
      }
    }
    vv.addEventListener('resize', onViewportChange)
    vv.addEventListener('scroll', onViewportChange)
    return () => {
      vv.removeEventListener('resize', onViewportChange)
      vv.removeEventListener('scroll', onViewportChange)
    }
  }, [fitRuntime])

  useEffect(() => {
    const refreshRenderer = () => {
      if (document.visibilityState !== 'visible') return
      refreshActiveRuntime()
    }
    document.addEventListener('visibilitychange', refreshRenderer)
    window.addEventListener('focus', refreshRenderer)
    return () => {
      document.removeEventListener('visibilitychange', refreshRenderer)
      window.removeEventListener('focus', refreshRenderer)
    }
  }, [refreshActiveRuntime])

  // Reconnect dead sockets when the tab resumes from background. Only fires
  // on visibilitychange (not focus) to avoid retriggering on every window
  // click, which defeats backoff on flaky connections (mobile / Tailscale).
  // Does not reset backoff — only socket.onopen should do that.
  useEffect(() => {
    const reconnectOnResume = () => {
      if (document.visibilityState !== 'visible') return
      const sessionName = activeSessionRef.current.trim()
      if (sessionName === '') return
      const runtime = runtimesRef.current.get(sessionName)
      if (!runtime) return
      if (isSocketOpenOrConnecting(runtime.socket)) return
      connectRuntime(runtime, { resetTerminal: false })
    }
    document.addEventListener('visibilitychange', reconnectOnResume)
    return () => {
      document.removeEventListener('visibilitychange', reconnectOnResume)
    }
  }, [connectRuntime])

  useEffect(() => {
    const RENDERER_REFRESH_MS = 5 * 60 * 1000
    const id = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return
      refreshActiveRuntime()
    }, RENDERER_REFRESH_MS)
    return () => window.clearInterval(id)
  }, [refreshActiveRuntime])

  useEffect(() => {
    const rafId = window.requestAnimationFrame(() => {
      fitTerminal()
    })
    return () => {
      window.cancelAnimationFrame(rafId)
    }
  }, [fitTerminal, sidebarCollapsed, activeSession])

  useEffect(() => {
    const runtimes = runtimesRef.current
    const hosts = hostsRef.current
    const hostCallbacks = hostCallbacksRef.current
    return () => {
      isMountedRef.current = false
      for (const runtime of [...runtimes.values()]) {
        disposeRuntime(runtime, 'detached')
      }
      runtimes.clear()
      hosts.clear()
      hostCallbacks.clear()
    }
  }, [disposeRuntime])

  const reconnectActiveSession = useCallback(
    (options?: { force?: boolean }) => {
      const sessionName = activeSessionRef.current.trim()
      if (sessionName === '') return
      const runtime = runtimesRef.current.get(sessionName)
      if (!runtime) return
      if (!options?.force && isSocketOpenOrConnecting(runtime.socket)) return
      runtime.reconnect.reset()
      connectRuntime(runtime, {
        force: options?.force === true,
        resetTerminal: false,
      })
    },
    [connectRuntime],
  )

  return {
    getTerminalHostRef,
    connectionState,
    statusDetail,
    termCols,
    termRows,
    setConnection,
    closeCurrentSocket,
    resetTerminal,
    fitTerminal,
    sendKey,
    flushComposition,
    focusTerminal,
    zoomIn,
    zoomOut,
    reconnectActiveSession,
  }
}
