// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RunbookStepDraft } from './RunbookStepEditor'
import { RunbookStepEditor } from './RunbookStepEditor'

afterEach(() => {
  cleanup()
})

function step(overrides: Partial<RunbookStepDraft> = {}): RunbookStepDraft {
  return {
    key: 'step-1',
    type: 'approval',
    title: 'Approve restart',
    command: '',
    script: '',
    description: '',
    continueOnError: false,
    timeout: '',
    retries: '',
    retryDelay: '',
    ...overrides,
  }
}

describe('RunbookStepEditor', () => {
  it('presents approval as an approval gate without command retry controls', () => {
    render(
      <RunbookStepEditor
        index={0}
        step={step()}
        errors={{ description: 'Description is required' }}
        isFirst={true}
        isLast={true}
        onChange={vi.fn()}
        onMoveUp={vi.fn()}
        onMoveDown={vi.fn()}
        onRemove={vi.fn()}
      />,
    )

    expect(screen.getByText('Approval instructions')).toBeTruthy()
    expect(screen.getByText('Description is required')).toBeTruthy()
    expect(screen.queryByText('Command')).toBeNull()
    expect(screen.queryByText('Script')).toBeNull()
    expect(screen.queryByText('Advanced')).toBeNull()
  })
})
