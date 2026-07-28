// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import SettingsSectionHeader from './SettingsSectionHeader'

afterEach(cleanup)

describe('SettingsSectionHeader', () => {
  it('stays accessible while remaining visually compact on mobile', () => {
    const { container } = render(
      <SettingsSectionHeader
        title="Storage"
        description="Usage and protected history"
        icon={<span aria-hidden="true">icon</span>}
      />,
    )

    const header = container.querySelector('header')
    const heading = screen.getByRole('heading', { name: 'Storage' })

    expect(header?.className).toContain('sr-only')
    expect(header?.className).toContain('md:not-sr-only')
    expect(header?.className).toContain('md:flex')
    expect(document.activeElement).toBe(heading)
  })
})
