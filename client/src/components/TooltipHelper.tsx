import type { ReactNode } from 'react'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface TooltipHelperProps {
  content: string
  side?: 'top' | 'right' | 'bottom' | 'left'
  children: ReactNode
}

export function TooltipHelper({
  content,
  side = 'bottom',
  children,
}: TooltipHelperProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side}>{content}</TooltipContent>
    </Tooltip>
  )
}
