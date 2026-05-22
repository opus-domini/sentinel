import { useEffect, useRef } from 'react'

type HotkeyBinding = {
  key: string
  ctrl?: boolean
  meta?: boolean
  alt?: boolean
  shift?: boolean
  enabled?: boolean
  ignoreEditable?: boolean
  allowTerminalTarget?: boolean
  preventDefault?: boolean
  stopPropagation?: boolean
  when?: (event: KeyboardEvent) => boolean
  handler: (event: KeyboardEvent) => void
}

type UseDocumentHotkeysOptions = {
  enabled?: boolean
  capture?: boolean
}

function normalizeKey(key: string): string {
  return key.toLowerCase()
}

function matchesHotkey(event: KeyboardEvent, binding: HotkeyBinding): boolean {
  if (normalizeKey(event.key) !== normalizeKey(binding.key)) {
    return false
  }

  if (binding.ctrl !== undefined && event.ctrlKey !== binding.ctrl) {
    return false
  }
  if (binding.meta !== undefined && event.metaKey !== binding.meta) {
    return false
  }
  if (binding.alt !== undefined && event.altKey !== binding.alt) {
    return false
  }
  if (binding.shift !== undefined && event.shiftKey !== binding.shift) {
    return false
  }

  return true
}

export function isEditableHotkeyTarget(
  target: EventTarget | null,
  { allowTerminalTarget = false }: { allowTerminalTarget?: boolean } = {},
): boolean {
  if (!(target instanceof Element)) {
    return false
  }

  if (allowTerminalTarget && target.closest('.xterm')) {
    return false
  }

  return target.closest('input, textarea, select, [contenteditable="true"]') !== null
}

export function useDocumentHotkeys(
  bindings: ReadonlyArray<HotkeyBinding>,
  { enabled = true, capture = true }: UseDocumentHotkeysOptions = {},
) {
  const bindingsRef = useRef(bindings)

  useEffect(() => {
    bindingsRef.current = bindings
  }, [bindings])

  useEffect(() => {
    if (!enabled) {
      return
    }

    const onKeyDown = (event: KeyboardEvent) => {
      for (const binding of bindingsRef.current) {
        if (binding.enabled === false || !matchesHotkey(event, binding)) {
          continue
        }
        if (
          binding.ignoreEditable !== false &&
          isEditableHotkeyTarget(event.target, {
            allowTerminalTarget: binding.allowTerminalTarget,
          })
        ) {
          continue
        }
        if (binding.when && !binding.when(event)) {
          continue
        }

        if (binding.preventDefault !== false) {
          event.preventDefault()
        }
        if (binding.stopPropagation !== false) {
          event.stopPropagation()
        }
        binding.handler(event)
        return
      }
    }

    document.addEventListener('keydown', onKeyDown, { capture })
    return () => {
      document.removeEventListener('keydown', onKeyDown, { capture })
    }
  }, [capture, enabled])
}
