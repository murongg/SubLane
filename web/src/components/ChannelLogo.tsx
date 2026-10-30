import { Plug } from 'lucide-react'
import type { ChannelPlatform } from '@/lib/channels'
import { cn } from '@/lib/cn'

const icons = import.meta.glob<string>('../assets/channels/*.svg', {
  eager: true,
  query: '?url',
  import: 'default',
})
const brands: Record<Exclude<ChannelPlatform, 'custom'>, string> = {
  openai: 'openai',
  anthropic: 'claude-color',
  gemini: 'gemini-color',
  deepseek: 'deepseek-color',
  openrouter: 'openrouter',
  opencode: 'opencode',
  'opencode-go': 'opencode',
  xai: 'xai',
  moonshot: 'kimi-color',
  'moonshot-cn': 'kimi-color',
  zhipu: 'zhipu-color',
  zai: 'zai',
  minimax: 'minimax-color',
  'minimax-cn': 'minimax-color',
  qwen: 'qwen-color',
  'qwen-cn': 'qwen-color',
  mistral: 'mistral-color',
  groq: 'groq',
  together: 'together-color',
  fireworks: 'fireworks-color',
  siliconflow: 'siliconcloud-color',
  'siliconflow-global': 'siliconcloud-color',
  nvidia: 'nvidia-color',
  modelscope: 'modelscope-color',
  azure: 'azure-color',
  volcengine: 'volcengine-color',
  baidu: 'baiducloud-color',
  tencent: 'tencentcloud-color',
  stepfun: 'stepfun-color',
  xiaomi: 'xiaomimimo',
}

export function ChannelLogo({
  platform,
  className,
}: {
  platform: ChannelPlatform
  className?: string
}) {
  if (platform === 'custom')
    return (
      <Plug aria-hidden="true" className={cn('size-6 shrink-0', className)} />
    )
  const name = brands[platform]
  return (
    <img
      src={icons[`../assets/channels/${name}.svg`]}
      alt=""
      aria-hidden="true"
      width={24}
      height={24}
      className={cn(
        'size-6 shrink-0 object-contain',
        !name.endsWith('-color') && 'dark:invert',
        className,
      )}
    />
  )
}
