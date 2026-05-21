import * as React from 'react'
import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const emptyStateVariants = cva(
  'select-none text-center text-muted-foreground',
  {
    variants: {
      variant: {
        default:
          'grid h-full place-items-center rounded-md border border-border-subtle bg-surface-sunken',
        inline: 'rounded-md border border-border bg-surface-overlay px-2 py-2',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

function EmptyState({
  className,
  variant = 'default',
  children,
  ...props
}: React.ComponentProps<'div'> & VariantProps<typeof emptyStateVariants>) {
  return (
    <div className={cn(emptyStateVariants({ variant, className }))} {...props}>
      {children}
    </div>
  )
}

export { EmptyState, emptyStateVariants }
