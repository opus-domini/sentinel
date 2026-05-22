// @vitest-environment jsdom
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ScrollArea } from './scroll-area'

describe('ScrollArea', () => {
  afterEach(() => {
    cleanup()
  })

  it('does not draw a focus ring around the whole scroll viewport', () => {
    const { container } = render(
      <ScrollArea>
        <button type="button">Focusable child</button>
      </ScrollArea>,
    )

    const viewport = container.querySelector('[data-slot="scroll-area-viewport"]')

    expect(viewport?.className).toContain('outline-none')
    expect(viewport?.className).not.toContain('focus-visible:ring')
    expect(viewport?.className).not.toContain('focus-visible:outline')
  })
})
