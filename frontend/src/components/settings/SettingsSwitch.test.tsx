// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SettingsSwitch from './SettingsSwitch'

describe('SettingsSwitch', () => {
  afterEach(cleanup)

  it('keeps a compact anchored track inside an accessible hit target', () => {
    const onCheckedChange = vi.fn()
    render(
      <SettingsSwitch
        label="Activity projection"
        checked={false}
        onCheckedChange={onCheckedChange}
      />,
    )

    const control = screen.getByRole('switch', { name: 'Activity projection' })
    const track = control.firstElementChild as HTMLElement
    const thumb = track.firstElementChild as HTMLElement

    expect(control.className).toContain('size-11')
    expect(track.className).toContain('h-5 w-9')
    expect(thumb.className).toContain('left-0.5')
    expect(thumb.className).not.toContain('translate-x-4')

    fireEvent.click(control)
    expect(onCheckedChange).toHaveBeenCalledWith(true)
  })

  it('moves the checked thumb within the compact warning track', () => {
    render(
      <SettingsSwitch
        label="Allow root targeting"
        checked
        tone="warning"
        onCheckedChange={vi.fn()}
      />,
    )

    const control = screen.getByRole('switch', { name: 'Allow root targeting' })
    const track = control.firstElementChild as HTMLElement
    const thumb = track.firstElementChild as HTMLElement

    expect(track.className).toContain('border-warning/60')
    expect(thumb.className).toContain('translate-x-4')
  })
})
