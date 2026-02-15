import ReactDOM from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import { isIOSDevice } from '@/lib/device'
import { registerSentinelPwa } from '@/lib/pwa'
import { queryClient } from '@/lib/queryClient'
import './styles.css'

if (isIOSDevice()) {
  document.documentElement.classList.add('ios')
}

const root = document.getElementById('root')
if (!root) {
  throw new Error('root element not found')
}

ReactDOM.createRoot(root).render(
  <QueryClientProvider client={queryClient}>
    <RouterProvider router={router} />
  </QueryClientProvider>,
)

registerSentinelPwa()
