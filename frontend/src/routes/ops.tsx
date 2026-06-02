import { Navigate, createFileRoute } from '@tanstack/react-router'

function OpsRedirect() {
  return <Navigate to="/services" replace />
}

export const Route = createFileRoute('/ops')({
  component: OpsRedirect,
})
