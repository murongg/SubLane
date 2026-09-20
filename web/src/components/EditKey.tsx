import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { LoaderCircle, Pencil } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { updateKey, type APIKey } from '@/lib/keys'
import { ApiError } from '@/lib/request'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { KeyExpiry } from './KeyExpiry'
import { expiryValue, type ExpiryChoice } from '@/lib/keys'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogTrigger,
} from './ui/Dialog'
export function EditKey({
  value,
  onSaved,
}: {
  value: APIKey
  onSaved: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label={t('editKeyNamed', { name: value.name })}
          title={t('editKey')}
          disabled={value.revoked_at !== null}
        >
          <Pencil aria-hidden="true" />
        </Button>
      </DialogTrigger>
      {open && (
        <KeyEditor
          value={value}
          onSaved={onSaved}
          onClose={() => setOpen(false)}
        />
      )}
    </Dialog>
  )
}
function KeyEditor({
  value,
  onSaved,
  onClose,
}: {
  value: APIKey
  onSaved: () => Promise<void>
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(value.name)
  const [enabled, setEnabled] = useState(value.enabled)
  const [expiry, setExpiry] = useState<ExpiryChoice>('keep')
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: updateKey,
    onSuccess: async () => {
      await onSaved()
      onClose()
    },
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (mutation.isPending) return
    const valid = name.trim().length > 0 && Array.from(name.trim()).length <= 64
    setInvalid(!valid)
    if (valid)
      mutation.mutate({
        id: value.id,
        name: name.trim(),
        enabled,
        expires_at: expiryValue(expiry, value.expires_at),
      })
  }
  return (
    <DialogContent
      showCloseButton={false}
      className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      onInteractOutside={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
      onEscapeKeyDown={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
    >
      <DialogHeader>
        <DialogTitle>{t('editKey')}</DialogTitle>
        <DialogDescription>{t('editKeyDescription')}</DialogDescription>
      </DialogHeader>
      <form onSubmit={submit} noValidate className="space-y-5">
        <div className="space-y-2">
          <label htmlFor="edit-key-name" className="text-sm font-medium">
            {t('keyName')}
          </label>
          <Input
            id="edit-key-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={128}
            disabled={mutation.isPending}
            aria-invalid={invalid}
          />
          {invalid && (
            <p role="alert" className="text-sm text-error">
              {t('keyNameHint')}
            </p>
          )}
        </div>
        <label className="flex items-center gap-3 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-primary focus-visible:ring-2 focus-visible:ring-ring"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            disabled={mutation.isPending}
          />
          {t('keyEnabled')}
        </label>
        <p className="text-xs leading-5 text-muted-foreground">
          {t('keyPauseHint')}
        </p>
        <KeyExpiry
          value={expiry}
          onChange={setExpiry}
          current={value.expires_at}
          disabled={mutation.isPending}
        />
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t(
              mutation.error instanceof ApiError &&
                mutation.error.code === 'api_key_revoked'
                ? 'keyAlreadyRevoked'
                : 'keyUpdateFailed',
            )}
          </p>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            {t('cancel')}
          </Button>
          <Button disabled={mutation.isPending} type="submit">
            {mutation.isPending && (
              <LoaderCircle
                className="motion-safe:animate-spin"
                aria-hidden="true"
              />
            )}
            {t('saveChanges')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
