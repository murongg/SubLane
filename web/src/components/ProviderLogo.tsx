import codex from '@/assets/providers/codex.svg'
import claude from '@/assets/providers/claude.svg'
import antigravity from '@/assets/providers/antigravity.svg'
import type { Provider } from '@/lib/accounts'
import { cn } from '@/lib/cn'

const logos: Record<Provider, string> = { codex, claude, antigravity }

export function ProviderLogo({
  provider,
  alt = '',
  className,
}: {
  provider: Provider
  alt?: string
  className?: string
}) {
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
