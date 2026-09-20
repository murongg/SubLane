import { useEffect, useId, useRef, useState } from 'react'
import { Check, Copy, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { APIKey } from '@/lib/keys'
import { useKeySecret } from '@/hooks/use-key-secret'
import { ApiError } from '@/lib/request'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function CopyKey({ value, userID }: { value: APIKey; userID: number }) {
  const { t } = useTranslation()
  const hintID = useId()
  const trigger = useRef<HTMLButtonElement>(null)
  const timer = useRef<number | undefined>(undefined)
  const [copied, setCopied] = useState(false)
  const [manualSecret, setManualSecret] = useState<string | null>(null)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  const { mutation } = useKeySecret(
    userID,
    value.id,
    async ({ secret, isCurrent }) => {
      try {
        await navigator.clipboard.writeText(secret)
        if (!isCurrent()) return
        setCopied(true)
        window.clearTimeout(timer.current)
        timer.current = window.setTimeout(() => setCopied(false), 2000)
      } catch {
        if (isCurrent()) setManualSecret(secret)
      }
    },
  )
  const legacy = !value.copyable && value.revoked_at === null
  const close = () => {
    setManualSecret(null)
    mutation.reset()
  }
  const Icon = mutation.isPending ? LoaderCircle : copied ? Check : Copy
  return (
    <div className="mt-1 space-y-1">
      <div className="flex items-center gap-1">
        <code className="text-xs text-muted-foreground">{value.prefix}…</code>
        <Button
          ref={trigger}
          type="button"
          variant="ghost"
          size="icon"
          className="size-7"
          disabled={!value.copyable || mutation.isPending}
          onClick={() => {
            setCopied(false)
            mutation.mutate()
          }}
          aria-label={t('copyKeyNamed', { name: value.name })}
          aria-describedby={legacy ? hintID : undefined}
          title={t(
            value.revoked_at !== null
              ? 'keyAlreadyRevoked'
              : legacy
                ? 'keyLegacyCopy'
                : copied
                  ? 'copied'
                  : 'copyKey',
          )}
        >
          <Icon
            aria-hidden="true"
            className={
              mutation.isPending
                ? 'size-3.5 motion-safe:animate-spin'
                : copied
                  ? 'size-3.5 text-success'
                  : 'size-3.5'
            }
          />
        </Button>
        <span role="status" className="sr-only">
          {copied ? t('copied') : mutation.isPending ? t('retrievingKey') : ''}
        </span>
      </div>
      {legacy && (
        <p id={hintID} className="text-xs leading-5 text-muted-foreground">
          {t('keyLegacyCopy')}
        </p>
      )}
      {mutation.isError && (
        <p role="alert" className="text-xs leading-5 text-error">
          {t(
            mutation.error instanceof ApiError &&
              mutation.error.code === 'api_key_not_copyable'
              ? 'keyLegacyCopy'
              : mutation.error instanceof ApiError &&
                  mutation.error.code === 'api_key_revoked'
                ? 'keyAlreadyRevoked'
                : 'keyRetrieveFailed',
          )}
        </p>
      )}
      <Dialog
        open={manualSecret !== null}
        onOpenChange={(open) => {
          if (!open) close()
        }}
      >
        {manualSecret !== null && (
          <DialogContent
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              trigger.current?.focus()
            }}
          >
            <DialogHeader>
              <DialogTitle>{t('copyKeyTitle')}</DialogTitle>
              <DialogDescription>{t('keyManualCopy')}</DialogDescription>
            </DialogHeader>
            <Input
              readOnly
              autoComplete="off"
              spellCheck={false}
              value={manualSecret}
              aria-label={t('apiKey')}
              className="font-mono text-sm"
              onFocus={(event) => event.currentTarget.select()}
            />
            <DialogFooter>
              <Button onClick={close}>{t('done')}</Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </div>
  )
}
