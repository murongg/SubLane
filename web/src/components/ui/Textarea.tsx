import type { ComponentProps } from 'react'
import { cn } from '@/lib/cn'

export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return (
    <textarea
      className={cn(
        'w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-2 text-base outline-none selection:bg-primary selection:text-primary-foreground placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-background',
        className,
      )}
      {...props}
    />
  )
}
