import { describe, expect, it } from 'vitest'

import { buildWSProtocols } from './wsAuth'

describe('buildWSProtocols', () => {
  it('always returns sentinel.v1 only', () => {
    expect(buildWSProtocols()).toEqual(['sentinel.v1'])
  })
})
