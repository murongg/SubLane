import { useId, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/cn'
import { Button } from './ui/Button'

export function RequestID({
  value,
  compact = false,
}: {
  value: string
  compact?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>(
    'idle',
  )
  if (!value) return <span title={t('requestIDMissing')}>—</span>
  const Icon = copyState === 'copied' ? Check : Copy
  return (
    <div className="min-w-0">
      <div className={cn('flex items-center gap-1', !compact && 'gap-2')}>
        <code
          id={id}
          title={value}
          className={cn(
            'min-w-0 select-all',
            compact
              ? 'max-w-[15ch] truncate text-xs text-muted-foreground'
              : 'flex-1 break-all text-sm',
          )}
        >
          {value}
        </code>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={compact ? 'size-7' : undefined}
          aria-label={t('requestCopyID')}
          aria-describedby={id}
          title={t(copyState === 'copied' ? 'copied' : 'requestCopyID')}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value)
              setCopyState('copied')
            } catch {
              setCopyState('failed')
            }
          }}
        >
          <Icon
            aria-hidden="true"
            className={compact ? 'size-3.5' : undefined}
          />
        </Button>
      </div>
      <p
        role="status"
        className={cn(
          'text-xs text-muted-foreground',
          compact && copyState !== 'failed'
            ? 'sr-only'
            : 'mt-1 max-w-64 whitespace-normal leading-5',
        )}
      >
        {copyState === 'copied'
          ? t('copied')
          : copyState === 'failed'
            ? t('requestCopyFailed')
            : ''}
      </p>
    </div>
  )
}
