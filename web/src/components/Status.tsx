import {
  CircleAlert,
  CircleCheck,
  CirclePause,
  Info,
  TriangleAlert,
} from 'lucide-react'
import { cn } from '@/lib/cn'

const variants = {
  success: { icon: CircleCheck, color: 'bg-success-muted text-success' },
  warning: { icon: TriangleAlert, color: 'bg-warning-muted text-warning' },
  error: { icon: CircleAlert, color: 'bg-error-muted text-error' },
  info: { icon: Info, color: 'bg-info-muted text-info' },
  neutral: { icon: CirclePause, color: 'bg-muted text-muted-foreground' },
}

export function Status({
  kind,
  children,
}: {
  kind: keyof typeof variants
  children: React.ReactNode
}) {
  const { icon: Icon, color } = variants[kind]
  return (
    <span
      className={cn(
        'inline-flex w-fit items-center gap-1.5 whitespace-nowrap rounded-md px-2 py-1 text-xs font-medium',
        color,
      )}
    >
      <Icon className="size-3.5" aria-hidden="true" />
      {children}
    </span>
  )
}
