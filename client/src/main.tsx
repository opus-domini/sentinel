import ReactDOM from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import { isIOSDevice } from '@/lib/device'
import './styles.css'

if (isIOSDevice()) {
  document.documentElement.classList.add('ios')
}

const root = document.getElementById('root')
if (!root) {
  throw new Error('root element not found')
}

ReactDOM.createRoot(root).render(<RouterProvider router={router} />)
