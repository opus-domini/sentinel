import type { IClipboardProvider } from '@xterm/addon-clipboard'

/**
 * Creates a clipboard provider for xterm.js ClipboardAddon that writes to
 * the system clipboard regardless of the OSC 52 selection type.
 *
 * The default BrowserClipboardProvider only handles selection type 'c'
 * (system clipboard) and silently ignores all others.  tmux emits OSC 52
 * with an empty selection string when `set-clipboard` is `on`, causing the
 * default provider to discard the clipboard write.  This provider accepts
 * any selection type so that tmux copy-mode, application OSC 52, and
 * primary selection all reach navigator.clipboard.
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
    writeText: async (_selection, text) => {
      try {
        await navigator.clipboard.writeText(text)
      } catch {
        // Clipboard write may fail without user gesture or permission.
      }
    },
  }
}
