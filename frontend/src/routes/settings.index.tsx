import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/')({
  beforeLoad: redirectToExperience,
})

export function redirectToExperience(): never {
  throw redirect({
    to: '/settings/$section',
    params: { section: 'experience' },
  })
}
