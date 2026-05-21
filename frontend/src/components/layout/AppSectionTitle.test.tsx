// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import AppSectionTitle from './AppSectionTitle'

describe('AppSectionTitle', () => {
  it('shows the current hostname before the section label', () => {
    render(
      <div className="flex">
        <AppSectionTitle hostname="drako" section="tmux" />
      </div>,
    )

    expect(screen.getByText('drako')).toBeTruthy()
    expect(screen.getByText('tmux')).toBeTruthy()
  })

  it('falls back to Sentinel when the hostname is unavailable', () => {
    render(
      <div className="flex">
        <AppSectionTitle hostname="" section="services" />
      </div>,
    )

    expect(screen.getByText('Sentinel')).toBeTruthy()
    expect(screen.getByText('services')).toBeTruthy()
  })
})
