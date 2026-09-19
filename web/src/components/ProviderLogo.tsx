import codex from '@/assets/providers/codex.svg'
import claude from '@/assets/providers/claude.svg'
import antigravity from '@/assets/providers/antigravity.svg'
import type { Provider } from '@/lib/accounts'

const logos: Record<Provider, string> = { codex, claude, antigravity }

export function ProviderLogo({ provider }: { provider: Provider }) {
  return (
    <img
      src={logos[provider]}
      alt=""
      aria-hidden="true"
      width={28}
      height={28}
      className="size-7 shrink-0 object-contain"
    />
  )
}
