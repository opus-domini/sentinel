import { Navigate, createFileRoute } from '@tanstack/react-router'

function OpsRedirect() {
  return <Navigate to="/alerts" replace />
}

export const Route = createFileRoute('/ops')({
  component: OpsRedirect,
})
