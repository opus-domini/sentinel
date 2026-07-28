// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SecretSettingControl from './SecretSettingControl'
import type { SensitiveSetting } from '@/api/settings'

const setting: SensitiveSetting = {
  configured: true,
  source: 'file',
  editable: true,
  applyMode: 'restart',
  restartPending: false,
  validation: { required: false },
}

describe('SecretSettingControl', () => {
  afterEach(cleanup)

  it('never renders an existing value and uses a write-only replacement input', () => {
    const onIntentChange = vi.fn()
    render(
      <SecretSettingControl
        id="token"
        label="Shared token"
        setting={setting}
        intent="keep"
        value=""
        placeholder="Enter a new token"
        onIntentChange={onIntentChange}
        onValueChange={vi.fn()}
      />,
    )

    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByText('Configured')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect(onIntentChange).toHaveBeenCalledWith('replace')
  })

  it('renders replacement with password and new-password semantics', () => {
    const onValueChange = vi.fn()
    render(
      <SecretSettingControl
        id="webhook"
        label="Webhook URL"
        setting={setting}
        intent="replace"
        value="new-value"
        placeholder="https://example.test/hook"
        onIntentChange={vi.fn()}
        onValueChange={onValueChange}
      />,
    )

    const input = screen.getByLabelText('New webhook url') as HTMLInputElement
    expect(input.type).toBe('password')
    expect(input.autocomplete).toBe('new-password')
    fireEvent.change(input, { target: { value: 'replacement' } })
    expect(onValueChange).toHaveBeenCalledWith('replacement')
  })

  it('makes environment-owned secrets read-only', () => {
    render(
      <SecretSettingControl
        id="token"
        label="Shared token"
        setting={{ ...setting, source: 'environment', editable: false }}
        intent="keep"
        value=""
        placeholder="Enter a new token"
        onIntentChange={vi.fn()}
        onValueChange={vi.fn()}
      />,
    )

    expect(screen.getByText(/owned by the environment/)).toBeTruthy()
    for (const button of screen.getAllByRole('button')) {
      expect((button as HTMLButtonElement).disabled).toBe(true)
    }
  })
})
