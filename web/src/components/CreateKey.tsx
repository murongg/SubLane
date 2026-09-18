import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Copy, LoaderCircle, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { createKey } from '@/lib/keys'
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
  DialogTrigger,
} from './ui/Dialog'

export function CreateKey({ onCreated }: { onCreated: () => Promise<void> }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Plus aria-hidden="true" />
          {t('createKey')}
        </Button>
      </DialogTrigger>
      {open && <KeyForm onClose={() => setOpen(false)} onCreated={onCreated} />}
    </Dialog>
  )
}

function KeyForm({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [nameError, setNameError] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copyFailed, setCopyFailed] = useState(false)
  // Only this mounted dialog holds the one-time secret; closing it releases mutation data immediately.
  const mutation = useMutation({
    mutationFn: createKey,
    gcTime: 0,
    onSuccess: onCreated,
  })
  const close = () => {
    mutation.reset()
    onClose()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const data = new FormData(event.currentTarget)
    const name = String(data.get('name') ?? '').trim()
    const characters = Array.from(name)
    const invalid =
      characters.length < 1 ||
      characters.length > 64 ||
      characters.some((character) => {
        const code = character.charCodeAt(0)
        return code < 32 || (code >= 127 && code <= 159)
      })
    setNameError(invalid)
    if (invalid) return
    mutation.mutate(name)
  }
  const copy = async () => {
    if (!mutation.data) return
    try {
      await navigator.clipboard.writeText(mutation.data.secret)
      setCopied(true)
      setCopyFailed(false)
    } catch {
      setCopyFailed(true)
    }
  }
  return (
    <DialogContent
      showCloseButton={false}
      onInteractOutside={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
      onEscapeKeyDown={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
    >
      <DialogHeader>
        <DialogTitle>
          {t(mutation.data ? 'saveKeyTitle' : 'createKeyTitle')}
        </DialogTitle>
        <DialogDescription>
          {t(mutation.data ? 'saveKeyDescription' : 'createKeyDescription')}
        </DialogDescription>
      </DialogHeader>
      {mutation.data ? (
        <>
          <Input
            readOnly
            value={mutation.data.secret}
            aria-label={t('apiKey')}
            autoComplete="off"
            className="font-mono text-sm"
            onFocus={(event) => event.currentTarget.select()}
          />
          {copyFailed && (
            <p role="alert" className="text-sm text-error">
              {t('copyKeyFailed')}
            </p>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => copy()}>
              <Copy aria-hidden="true" />
              {t(copied ? 'copied' : 'copyKey')}
            </Button>
            <Button onClick={close}>{t('done')}</Button>
          </DialogFooter>
        </>
      ) : (
        <form onSubmit={submit} noValidate className="space-y-5">
          <div className="space-y-2">
            <label htmlFor="key-name" className="text-sm font-medium">
              {t('keyName')}
            </label>
            <Input
              id="key-name"
              name="name"
              maxLength={128}
              autoComplete="off"
              required
              disabled={mutation.isPending}
              aria-invalid={nameError}
              aria-describedby={nameError ? 'key-name-error' : undefined}
            />
            {nameError && (
              <p
                id="key-name-error"
                role="alert"
                className="text-sm text-error"
              >
                {t('keyNameHint')}
              </p>
            )}
          </div>
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                mutation.error instanceof ApiError &&
                  mutation.error.code === 'api_key_limit'
                  ? 'keyLimitReached'
                  : 'keyCreateFailed',
              )}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={close}
              disabled={mutation.isPending}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t('createKey')}
            </Button>
          </DialogFooter>
        </form>
      )}
    </DialogContent>
  )
}
