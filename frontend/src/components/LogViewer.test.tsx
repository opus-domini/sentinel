// @vitest-environment jsdom
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LogViewer } from './LogViewer'
import { parseLogLines } from '@/lib/log-parser'

describe('LogViewer', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(private readonly callback: ResizeObserverCallback) {}

        observe(target: Element) {
          this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver)
        }

        unobserve() {}

        disconnect() {}
      },
    )
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(160)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      bottom: 16,
      height: 16,
      left: 0,
      right: 800,
      top: 0,
      width: 800,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('keeps the rendered row count bounded for large log buffers', async () => {
    const lines = parseLogLines(
      Array.from({ length: 1_000 }, (_, index) => `line ${index + 1}`).join('\n'),
    )
    const { container } = render(
      <LogViewer
        lines={lines}
        loading={false}
        searchQuery=""
        wordWrap={false}
        follow={false}
        onFollowChange={() => {}}
      />,
    )

    await waitFor(() => {
      const renderedRows = container.querySelectorAll('[data-index]').length
      expect(renderedRows).toBeGreaterThan(0)
      expect(renderedRows).toBeLessThan(100)
    })
  })
})
