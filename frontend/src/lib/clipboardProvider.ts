import type { IClipboardProvider } from '@xterm/addon-clipboard'

/**
 * Write text to the system clipboard with a fallback for non-secure contexts
 * (plain HTTP over IP) where navigator.clipboard is unavailable. The fallback
 * uses a temporary textarea + execCommand('copy'), which works when triggered
 * by a user gesture (e.g. mouse selection).
 */
export function writeClipboardText(text: string): void {
  // navigator.clipboard is undefined in non-secure contexts (HTTP over IP).
  // TypeScript DOM types declare it as always present, so we cast to check.
  const clipboard = navigator.clipboard as Clipboard | undefined
  if (clipboard) {
    clipboard.writeText(text).catch(() => {})
    return
  }
  try {
    const el = document.createElement('textarea')
    el.value = text
    el.style.position = 'fixed'
    el.style.left = '-9999px'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    document.execCommand('copy')
    document.body.removeChild(el)
  } catch {
    // execCommand fallback may fail without user gesture.
  }
}

/**
 * Creates a clipboard provider for xterm.js ClipboardAddon that writes to
 * the system clipboard regardless of the OSC 52 selection type.
 *
 * The default BrowserClipboardProvider only handles selection type 'c'
 * (system clipboard) and silently ignores all others.  tmux emits OSC 52
 * with an empty selection string when `set-clipboard` is `on`, causing the
 * default provider to discard the clipboard write.  This provider accepts
 * any selection type so that tmux copy-mode, application OSC 52, and
 * primary selection all reach the clipboard.
 */
export function createWebClipboardProvider(): IClipboardProvider {
  return {
    readText: async () => {
      try {
        return await navigator.clipboard.readText()
      } catch {
        return ''
      }
    },
    writeText: (_selection, text) => {
      writeClipboardText(text)
      return Promise.resolve()
    },
  }
}
