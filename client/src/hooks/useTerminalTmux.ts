import { useCallback, useEffect, useRef, useState } from 'react'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { SerializeAddon } from '@xterm/addon-serialize'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { WebglAddon } from '@xterm/addon-webgl'
import { Terminal } from '@xterm/xterm'
import type { RefCallback } from 'react'
import type { ConnectionState } from '../types'
import { useIsMobileLayout } from '@/hooks/useIsMobileLayout'
import { attachTouchWheelBridge } from '@/lib/touchWheelBridge'
import { THEME_STORAGE_KEY, getTerminalTheme } from '@/lib/terminalThemes'
import { buildWSProtocols } from '@/lib/wsAuth'

const MIN_FONT_SIZE = 8
const MAX_FONT_SIZE = 24
const FONT_SIZE_KEY = 'sentinel_font_size'

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
  isComposing: boolean
  onDataDispose: Disposable
  onResizeDispose: Disposable
  contextMenuDispose: Disposable
  touchWheelDispose: Disposable
  webglContextLossDispose: Disposable
  hostResizeObserver: ResizeObserver | null
  hostResizeRafId: number | null
}

type UseTerminalTmuxArgs = {
  openTabs: Array<string>
  activeSession: string
  activeEpoch: number
  token: string
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
}

export function useTerminalTmux({
  openTabs,
  activeSession,
  activeEpoch,
  token,
  sidebarCollapsed,
  onAttachedMobile,
  wsPath = '/ws/tmux',
  wsQueryKey = 'session',
  connectingVerb = 'opening',
  connectedVerb = 'attached',
  allowWheelInAlternateBuffer = false,
  suppressBrowserContextMenu = false,
}: UseTerminalTmuxArgs): UseTerminalTmuxResult {
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('disconnected')
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
  const hostCallbacksRef = useRef(
    new Map<string, RefCallback<HTMLDivElement>>(),
  )

  const setRuntimeStatus = useCallback(
    (runtime: SessionRuntime, next: ConnectionState, detail: string) => {
      runtime.connectionState = next
      runtime.statusDetail = detail
      if (
        !isMountedRef.current ||
        activeSessionRef.current !== runtime.session
      ) {
        return
      }
      setConnectionState(next)
      setStatusDetail(detail)
    },
    [],
  )

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

  const sendResize = useCallback(
    (runtime: SessionRuntime, cols: number, rows: number) => {
      if (cols <= 0 || rows <= 0) {
        return
      }
      const socket = runtime.socket
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        return
      }
      socket.send(JSON.stringify({ type: 'resize', cols, rows }))
    },
    [],
  )

  const fitRuntime = useCallback(
    (runtime: SessionRuntime) => {
      if (!runtime.terminal.element) {
        return
      }
      runtime.fitAddon.fit()
      runtime.cols = runtime.terminal.cols
      runtime.rows = runtime.terminal.rows
      sendResize(runtime, runtime.cols, runtime.rows)

      if (
        !isMountedRef.current ||
        activeSessionRef.current !== runtime.session
      ) {
        return
      }
      setTermCols(runtime.cols)
      setTermRows(runtime.rows)
    },
    [sendResize],
  )

  const cleanupHostResizeObserver = useCallback((runtime: SessionRuntime) => {
    runtime.hostResizeObserver?.disconnect()
    runtime.hostResizeObserver = null
    if (runtime.hostResizeRafId !== null) {
      window.cancelAnimationFrame(runtime.hostResizeRafId)
      runtime.hostResizeRafId = null
    }
  }, [])

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
      observeHostResize(runtime, host)
      if (runtime.terminal.element) {
        runtime.touchWheelDispose.dispose()
        runtime.touchWheelDispose = { dispose: () => undefined }
        if (isMobileRef.current && allowWheelInAlternateBuffer) {
          runtime.touchWheelDispose = attachTouchWheelBridge({
            host,
            dispatchTarget: runtime.terminal.element,
          })
        }
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
          const dispatchTarget = host.querySelector<HTMLElement>('.xterm')
          runtime.touchWheelDispose = attachTouchWheelBridge({
            host,
            dispatchTarget: dispatchTarget ?? host,
          })
        }

        // FitAddon floors column/row counts, so a residual strip can appear.
        // Keep the xterm root in sync with theme background to hide it.
        const themeBg = getTerminalTheme(themeId).colors.background ?? ''
        host.style.setProperty('background-color', themeBg)

        fitRuntime(runtime)
      }

      void document.fonts.ready.then(openTerminal).catch(openTerminal)
    },
    [
      fitRuntime,
      observeHostResize,
      suppressBrowserContextMenu,
      themeId,
      allowWheelInAlternateBuffer,
    ],
  )

  const isSocketCurrent = useCallback(
    (
      runtime: SessionRuntime,
      generation: number,
      socket: WebSocket,
    ): boolean => {
      return (
        runtimesRef.current.get(runtime.session) === runtime &&
        runtime.generation === generation &&
        runtime.socket === socket
      )
    },
    [],
  )

  const isRuntimeCurrent = useCallback(
    (runtime: SessionRuntime, generation: number): boolean => {
      return (
        runtimesRef.current.get(runtime.session) === runtime &&
        runtime.generation === generation
      )
    },
    [],
  )

  const connectRuntime = useCallback(
    (runtime: SessionRuntime, options?: { resetTerminal?: boolean }) => {
      runtime.generation += 1
      const generation = runtime.generation

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
      setRuntimeStatus(
        runtime,
        'connecting',
        `${connectingVerb} ${runtime.session}`,
      )

      const wsURL = new URL(wsPath, window.location.origin)
      wsURL.searchParams.set(wsQueryKey, runtime.session)
      const socket = new WebSocket(
        wsURL.toString().replace(/^http/, 'ws'),
        buildWSProtocols(token),
      )
      socket.binaryType = 'arraybuffer'
      runtime.socket = socket

      socket.onopen = () => {
        if (!isSocketCurrent(runtime, generation, socket)) {
          return
        }
        runtime.manualCloseReason = null
        setRuntimeStatus(
          runtime,
          'connected',
          `${connectedVerb} ${runtime.session}`,
        )
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
              setRuntimeStatus(
                runtime,
                'error',
                message.message ?? 'terminal error',
              )
            }
          } catch {
            // ignore invalid control frame
          }
          return
        }

        if (event.data instanceof ArrayBuffer) {
          runtime.terminal.write(new Uint8Array(event.data))
          return
        }

        if (event.data instanceof Blob) {
          void event.data.arrayBuffer().then((buffer) => {
            if (!isSocketCurrent(runtime, generation, socket)) {
              return
            }
            runtime.terminal.write(new Uint8Array(buffer))
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
        if (!isRuntimeCurrent(runtime, generation)) {
          return
        }
        if (runtime.socket === socket) {
          runtime.socket = null
        }
        const reason = runtime.manualCloseReason ?? 'connection closed'
        runtime.manualCloseReason = null
        setRuntimeStatus(runtime, 'disconnected', reason)
      }
    },
    [
      connectedVerb,
      connectingVerb,
      fitRuntime,
      isRuntimeCurrent,
      isSocketCurrent,
      onAttachedMobile,
      setRuntimeStatus,
      token,
      wsPath,
      wsQueryKey,
    ],
  )

  const closeRuntimeSocket = useCallback(
    (runtime: SessionRuntime, reason?: string) => {
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
    [setRuntimeStatus],
  )

  const createRuntime = useCallback(
    (session: string): SessionRuntime => {
      const terminal = new Terminal({
        cursorBlink: true,
        fontFamily:
          'JetBrains Mono Variable, JetBrains Mono, SF Mono, monospace',
        fontSize,
        lineHeight: 1,
        scrollback: 5000,
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
      const clipboardAddon = new ClipboardAddon()
      const searchAddon = new SearchAddon({ highlightLimit: 2_000 })
      const serializeAddon = new SerializeAddon()
      const webLinksAddon = new WebLinksAddon((event, uri) => {
        event.preventDefault()
        window.open(uri, '_blank', 'noopener,noreferrer')
      })
      const webglAddon = new WebglAddon()

      terminal.loadAddon(fitAddon)
      terminal.loadAddon(clipboardAddon)
      terminal.loadAddon(searchAddon)
      terminal.loadAddon(serializeAddon)
      terminal.loadAddon(webLinksAddon)
      terminal.loadAddon(webglAddon)

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
        isComposing: false,
        onDataDispose: { dispose: () => undefined },
        onResizeDispose: { dispose: () => undefined },
        contextMenuDispose: { dispose: () => undefined },
        touchWheelDispose: { dispose: () => undefined },
        webglContextLossDispose: webglAddon.onContextLoss(() => {
          console.warn(`sentinel: webgl context lost (${session})`)
        }),
        hostResizeObserver: null,
        hostResizeRafId: null,
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

        if (
          !isMountedRef.current ||
          activeSessionRef.current !== runtime.session
        ) {
          return
        }
        setTermCols(cols)
        setTermRows(rows)
      })

      runtimesRef.current.set(session, runtime)

      const host = hostsRef.current.get(session)
      if (host) {
        openRuntimeInHost(runtime, host)
      }

      connectRuntime(runtime)
      return runtime
    },
    [
      allowWheelInAlternateBuffer,
      connectRuntime,
      fontSize,
      openRuntimeInHost,
      sendResize,
      themeId,
    ],
  )

  const disposeRuntime = useCallback(
    (runtime: SessionRuntime, reason: string) => {
      runtime.generation += 1
      closeRuntimeSocket(runtime, reason)
      cleanupHostResizeObserver(runtime)
      runtime.onDataDispose.dispose()
      runtime.onResizeDispose.dispose()
      runtime.contextMenuDispose.dispose()
      runtime.touchWheelDispose.dispose()
      runtime.webglContextLossDispose.dispose()
      runtime.terminal.dispose()
      runtimesRef.current.delete(runtime.session)
      hostsRef.current.delete(runtime.session)
      hostCallbacksRef.current.delete(runtime.session)
    },
    [cleanupHostResizeObserver, closeRuntimeSocket],
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
    runtime?.terminal.reset()
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
        if (runtime.terminal.element) {
          runtime.fitAddon.fit()
          runtime.cols = runtime.terminal.cols
          runtime.rows = runtime.terminal.rows
          sendResize(runtime, runtime.cols, runtime.rows)
        }
      }

      const activeRuntime = runtimesRef.current.get(
        activeSessionRef.current.trim(),
      )
      if (activeRuntime && isMountedRef.current) {
        setTermCols(activeRuntime.cols)
        setTermRows(activeRuntime.rows)
      }
    },
    [sendResize],
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

  const applyTheme = useCallback((id: string) => {
    setThemeId(id)
    const colors = getTerminalTheme(id).colors
    for (const runtime of runtimesRef.current.values()) {
      runtime.terminal.options.theme = colors
      if (runtime.terminal.element) {
        runtime.terminal.element.style.backgroundColor = colors.background ?? ''
      }
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

  useEffect(() => {
    activeSessionRef.current = activeSession
    publishActiveRuntimeState()
    fitTerminal()

    const runtime = runtimesRef.current.get(activeSession.trim())
    if (!runtime) {
      return
    }

    if (!runtime.socket || runtime.connectionState === 'error') {
      connectRuntime(runtime, { resetTerminal: false })
    }

    if (!isMobileRef.current) {
      runtime.terminal.focus()
    }
  }, [
    activeEpoch,
    activeSession,
    connectRuntime,
    fitTerminal,
    publishActiveRuntimeState,
  ])

  useEffect(() => {
    const allowedSessions = new Set(
      openTabs.filter((session) => session.trim() !== ''),
    )

    for (const session of allowedSessions) {
      if (!runtimesRef.current.has(session)) {
        createRuntime(session)
      }
    }

    for (const runtime of [...runtimesRef.current.values()]) {
      if (!allowedSessions.has(runtime.session)) {
        disposeRuntime(runtime, 'tab closed')
      }
    }

    publishActiveRuntimeState()
  }, [createRuntime, disposeRuntime, openTabs, publishActiveRuntimeState])

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
      runtime.touchWheelDispose = attachTouchWheelBridge({
        host,
        dispatchTarget: terminalElement,
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
    const rafId = window.requestAnimationFrame(() => {
      fitTerminal()
    })
    return () => {
      window.cancelAnimationFrame(rafId)
    }
  }, [fitTerminal, sidebarCollapsed, activeSession])

  useEffect(() => {
    return () => {
      isMountedRef.current = false
      for (const runtime of [...runtimesRef.current.values()]) {
        disposeRuntime(runtime, 'detached')
      }
      runtimesRef.current.clear()
      hostsRef.current.clear()
      hostCallbacksRef.current.clear()
    }
  }, [disposeRuntime])

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
  }
}
