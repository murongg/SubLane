import codex from '@/assets/providers/codex.svg'
import claude from '@/assets/providers/claude.svg'
import antigravity from '@/assets/providers/antigravity.svg'
import type { Provider } from '@/lib/accounts'
import { Plug } from 'lucide-react'
import { cn } from '@/lib/cn'

const logos: Record<Exclude<Provider, 'xai' | 'openai'>, string> = {
  codex,
  claude,
  antigravity,
}

export function ProviderLogo({
  provider,
  alt = '',
  className,
}: {
  provider: Provider
  alt?: string
  className?: string
}) {
  if (provider === 'openai')
    return (
      <Plug
        role={alt ? 'img' : undefined}
        aria-label={alt || undefined}
        aria-hidden={alt ? undefined : true}
        className={cn('size-7 shrink-0', className)}
      />
    )
  if (provider === 'xai')
    return (
      <span
        role={alt ? 'img' : undefined}
        aria-label={alt || undefined}
        aria-hidden={alt ? undefined : true}
        title={alt || undefined}
        className={cn(
          'inline-flex size-7 shrink-0 items-center justify-center text-lg font-semibold tracking-tighter',
          className,
        )}
      >
        xAI
      </span>
    )
  return (
    <img
      src={logos[provider]}
      alt={alt}
      title={alt || undefined}
      aria-hidden={alt ? undefined : true}
      width={28}
      height={28}
      className={cn('size-7 shrink-0 object-contain', className)}
    />
  )
}
