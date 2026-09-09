// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { Route } from './__root'

describe('root route error component', () => {
  afterEach(cleanup)

  it.each([
    [new Error('Unable to load route'), 'Unable to load route'],
    [null, 'An unexpected error occurred'],
    ['unexpected rejection', 'An unexpected error occurred'],
    [{ message: 'not an Error' }, 'An unexpected error occurred'],
  ])('renders a safe message for %j', (error, message) => {
    const ErrorComponent = Route.options.errorComponent
    if (!ErrorComponent) throw new Error('Root route error component is missing')

    render(<ErrorComponent error={error} reset={() => {}} />)

    expect(screen.getByRole('heading', { name: 'Error' })).not.toBeNull()
    expect(screen.getByText(message)).not.toBeNull()
  })
})
