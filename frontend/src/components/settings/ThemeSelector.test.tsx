// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ThemeSelector from './ThemeSelector'
import { terminalThemes } from '@/lib/terminalThemes'

describe('ThemeSelector', () => {
  afterEach(() => {
    cleanup()
  })

  it('exposes the active terminal theme as pressed', () => {
    const activeTheme = terminalThemes[0]

    render(<ThemeSelector activeThemeId={activeTheme.id} onSelect={() => {}} />)

    expect(
      screen.getByRole('button', { name: activeTheme.label }).getAttribute('aria-pressed'),
    ).toBe('true')
  })

  it('selects a terminal theme by id', () => {
    const onSelect = vi.fn()
    const nextTheme = terminalThemes[1]

    render(<ThemeSelector activeThemeId={terminalThemes[0].id} onSelect={onSelect} />)

    fireEvent.click(screen.getByRole('button', { name: nextTheme.label }))

    expect(onSelect).toHaveBeenCalledWith(nextTheme.id)
  })

  it('keeps the ANSI palette shrinkable inside narrow theme cards', () => {
    const activeTheme = terminalThemes[0]

    render(<ThemeSelector activeThemeId={activeTheme.id} onSelect={() => {}} />)

    const card = screen.getByRole('button', { name: activeTheme.label })
    const palette = card.querySelector('[aria-hidden="true"]')
    const swatches = palette?.querySelectorAll('span')

    expect(palette?.className).toContain('w-full')
    expect(palette?.className).toContain('grid-cols-8')
    expect(swatches).toHaveLength(8)
    expect(swatches?.[0].className).toContain('w-full')
  })
})
