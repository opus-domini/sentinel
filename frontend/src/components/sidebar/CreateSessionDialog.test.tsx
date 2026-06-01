// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import CreateSessionDialog from './CreateSessionDialog'

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    processUser: 'hugo',
    isRoot: false,
    canSwitchUser: true,
    allowedUsers: ['hugo', 'postgres'],
  }),
}))

describe('CreateSessionDialog', () => {
  const originalFetch = globalThis.fetch

  beforeEach(() => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: { dirs: [] } }),
    }) as typeof globalThis.fetch
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    cleanup()
  })

  it('shows advanced options toggle when canSwitchUser is true', () => {
    render(<CreateSessionDialog open onOpenChange={vi.fn()} defaultCwd="" onCreate={vi.fn()} />)

    expect(screen.getByText('Advanced options')).toBeTruthy()
  })

  it('closes and resets only after create resolves', async () => {
    const onOpenChange = vi.fn()
    let resolveCreate!: () => void
    const onCreate = vi.fn().mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveCreate = resolve
        }),
    )

    render(
      <CreateSessionDialog open onOpenChange={onOpenChange} defaultCwd="" onCreate={onCreate} />,
    )

    fireEvent.change(screen.getByPlaceholderText('session name'), {
      target: { value: 'dev' },
    })
    fireEvent.change(screen.getByPlaceholderText('working directory'), {
      target: { value: '/tmp' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    expect(onCreate).toHaveBeenCalledWith('dev', '/tmp', undefined)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    resolveCreate()
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  it('keeps values and shows an inline error when create rejects', async () => {
    const onOpenChange = vi.fn()
    const onCreate = vi.fn().mockRejectedValue(new Error('Create failed'))

    render(
      <CreateSessionDialog open onOpenChange={onOpenChange} defaultCwd="" onCreate={onCreate} />,
    )

    const name = screen.getByPlaceholderText('session name')
    const cwd = screen.getByPlaceholderText('working directory')
    fireEvent.change(name, { target: { value: 'dev' } })
    fireEvent.change(cwd, { target: { value: '/tmp' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    expect((await screen.findByRole('alert')).textContent).toContain('Create failed')
    expect((name as HTMLInputElement).value).toBe('dev')
    expect((cwd as HTMLInputElement).value).toBe('/tmp')
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })

  it('sets aria-activedescendant to the active directory option', async () => {
    globalThis.fetch = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/api/fs/dirs')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { dirs: ['/work/app', '/work/api'] } }),
        })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ data: { dirs: [] } }) })
    }) as typeof globalThis.fetch

    render(<CreateSessionDialog open onOpenChange={vi.fn()} defaultCwd="" onCreate={vi.fn()} />)

    const cwd = screen.getByRole('combobox', { name: 'Working directory' })
    fireEvent.focus(cwd)
    fireEvent.change(cwd, { target: { value: '/work' } })

    const option = await screen.findByRole('option', { name: '/work/app' })
    fireEvent.keyDown(cwd, { key: 'ArrowDown' })

    expect(cwd.getAttribute('aria-activedescendant')).toBe(option.id)
    expect(option.getAttribute('aria-selected')).toBe('true')
  })
})
