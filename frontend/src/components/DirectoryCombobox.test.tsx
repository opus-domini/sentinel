// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import DirectoryCombobox from './DirectoryCombobox'

function Harness({
  initial = '',
  fallbackPrefix,
  open,
}: {
  initial?: string
  fallbackPrefix?: string
  open?: boolean
}) {
  const [value, setValue] = useState(initial)
  return (
    <DirectoryCombobox
      value={value}
      onChange={setValue}
      ariaLabel="dir"
      fallbackPrefix={fallbackPrefix}
      open={open}
    />
  )
}

function mockDirs(dirs: Array<string>) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ data: { dirs } }),
  }) as typeof globalThis.fetch
}

describe('DirectoryCombobox', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    cleanup()
  })

  it('disables browser-native autocomplete in favor of directory suggestions', () => {
    render(<Harness />)

    expect(screen.getByRole('combobox', { name: 'dir' }).getAttribute('autocomplete')).toBe('off')
  })

  it('fetches and lists directory suggestions while focused', async () => {
    mockDirs(['/work/app', '/work/api'])

    render(<Harness />)
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: '/work' } })

    expect(await screen.findByRole('option', { name: '/work/app' })).toBeTruthy()
    expect(screen.getByRole('option', { name: '/work/api' })).toBeTruthy()
  })

  it('applies a suggestion to the value when selected', async () => {
    mockDirs(['/work/app'])

    render(<Harness initial="/work" />)
    const input = screen.getByRole('combobox', { name: 'dir' }) as HTMLInputElement
    fireEvent.focus(input)

    const option = await screen.findByRole('option', { name: '/work/app' })
    fireEvent.mouseDown(option)

    expect(input.value).toBe('/work/app')
  })

  it('does not query when the value is empty and no fallback prefix is set', () => {
    const fetchMock = vi.fn()
    globalThis.fetch = fetchMock as typeof globalThis.fetch

    render(<Harness />)

    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('queries the fallback prefix when the value is empty', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: { dirs: [] } }),
    })
    globalThis.fetch = fetchMock as typeof globalThis.fetch

    render(<Harness fallbackPrefix="/home" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(String(fetchMock.mock.calls[0][0])).toContain(`prefix=${encodeURIComponent('/home')}`)
  })

  it('accepts the highlighted suggestion on Enter, navigating with arrows', async () => {
    mockDirs(['/work/app', '/work/api'])

    render(<Harness initial="/work" />)
    const input = screen.getByRole('combobox', { name: 'dir' }) as HTMLInputElement
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    fireEvent.keyDown(input, { key: 'ArrowDown' }) // -> /work/app
    fireEvent.keyDown(input, { key: 'ArrowDown' }) // -> /work/api
    fireEvent.keyDown(input, { key: 'ArrowUp' }) // -> /work/app
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(input.value).toBe('/work/app')
  })

  it('accepts the sole suggestion on Tab', async () => {
    mockDirs(['/work/app'])

    render(<Harness initial="/work" />)
    const input = screen.getByRole('combobox', { name: 'dir' }) as HTMLInputElement
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    fireEvent.keyDown(input, { key: 'Tab' })

    expect(input.value).toBe('/work/app')
  })

  it('closes the listbox on Escape', async () => {
    mockDirs(['/work/app'])

    render(<Harness initial="/work" />)
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    fireEvent.keyDown(input, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('option', { name: '/work/app' })).toBeNull()
    })
    expect(input.getAttribute('aria-controls')).toBeNull()
  })

  it('closes the listbox on a pointer press outside the field', async () => {
    mockDirs(['/work/app'])

    render(
      <div>
        <Harness initial="/work" />
        <button type="button">outside</button>
      </div>,
    )
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    fireEvent.pointerDown(screen.getByRole('button', { name: 'outside' }))

    await waitFor(() => {
      expect(screen.queryByRole('option', { name: '/work/app' })).toBeNull()
    })
  })

  it('never shows the listbox or queries the server while closed', () => {
    const fetchMock = vi.fn()
    globalThis.fetch = fetchMock as typeof globalThis.fetch

    render(<Harness initial="/work" open={false} />)
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: '/work/app' } })

    expect(fetchMock).not.toHaveBeenCalled()
    expect(screen.queryByRole('option')).toBeNull()
    expect(input.getAttribute('aria-expanded')).toBe('false')
  })

  it('keeps autocomplete working after an outside press while still focused', async () => {
    mockDirs(['/work/app'])

    render(
      <div>
        <Harness initial="/work" />
        <button type="button">outside</button>
      </div>,
    )
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    // An outside press hides the listbox but must not drop focus...
    fireEvent.pointerDown(screen.getByRole('button', { name: 'outside' }))
    await waitFor(() => {
      expect(screen.queryByRole('option', { name: '/work/app' })).toBeNull()
    })

    // ...so continued typing re-opens suggestions without re-focusing.
    fireEvent.change(input, { target: { value: '/work/a' } })
    expect(await screen.findByRole('option', { name: '/work/app' })).toBeTruthy()
  })

  it('does not reopen the listbox when an outside control changes the value', async () => {
    mockDirs(['/picked/child'])

    function ChipHarness() {
      const [value, setValue] = useState('/work')
      return (
        <div>
          <DirectoryCombobox value={value} onChange={setValue} ariaLabel="dir" />
          <button type="button" onClick={() => setValue('/picked')}>
            chip
          </button>
        </div>
      )
    }

    render(<ChipHarness />)
    const input = screen.getByRole('combobox', { name: 'dir' }) as HTMLInputElement
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/picked/child' })

    // Tap an outside chip: pointerdown hides + suppresses, click sets a new value
    // (which schedules a debounced refetch) — mirrors iOS Safari, where the button
    // press does not blur the still-focused input.
    const chip = screen.getByRole('button', { name: 'chip' })
    fireEvent.pointerDown(chip)
    fireEvent.click(chip)

    // Let the value-driven refetch fully resolve.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 200))
    })

    // The listbox must stay closed even though suggestions were refetched.
    expect(input.value).toBe('/picked')
    expect(screen.queryByRole('option')).toBeNull()

    // A keystroke lifts the suppression and reopens suggestions.
    fireEvent.change(input, { target: { value: '/picked/' } })
    expect(await screen.findByRole('option', { name: '/picked/child' })).toBeTruthy()
  })

  it('does not accept a hidden suggestion via Tab while suppressed', async () => {
    mockDirs(['/picked/only'])

    function ChipHarness() {
      const [value, setValue] = useState('/work')
      return (
        <div>
          <DirectoryCombobox value={value} onChange={setValue} ariaLabel="dir" />
          <button type="button" onClick={() => setValue('/picked')}>
            chip
          </button>
        </div>
      )
    }

    render(<ChipHarness />)
    const input = screen.getByRole('combobox', { name: 'dir' }) as HTMLInputElement
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/picked/only' })

    const chip = screen.getByRole('button', { name: 'chip' })
    fireEvent.pointerDown(chip)
    fireEvent.click(chip)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 200))
    })
    // Sole suggestion is refetched but hidden (suppressed).
    expect(screen.queryByRole('option')).toBeNull()

    // Tab must leave focus/value alone, not auto-complete the hidden suggestion.
    fireEvent.keyDown(input, { key: 'Tab' })
    expect(input.value).toBe('/picked')
    expect(screen.queryByRole('option')).toBeNull()
  })

  it('keeps suggestions working when refocused within the blur delay', async () => {
    mockDirs(['/work/app'])

    render(<Harness initial="/work" />)
    const input = screen.getByRole('combobox', { name: 'dir' })
    fireEvent.focus(input)
    await screen.findByRole('option', { name: '/work/app' })

    // Blur then refocus within the 80ms window: the stale close timer must be cancelled
    // so `focused` is not clobbered to false after the delay elapses.
    fireEvent.blur(input)
    fireEvent.focus(input)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 120))
    })

    fireEvent.change(input, { target: { value: '/work/a' } })
    expect(await screen.findByRole('option', { name: '/work/app' })).toBeTruthy()
  })
})
