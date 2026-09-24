import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { proxyErrorKey, saveProxy, type Proxy } from '@/lib/proxies'
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

export function ProxyEditor({
  proxy,
  onClose,
  onSaved,
  restoreFocus,
}: {
  proxy?: Proxy
  onClose: () => void
  onSaved: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(proxy?.name ?? '')
  const [url, setURL] = useState('')
  const [invalid, setInvalid] = useState(false)
  const save = useMutation({
    mutationFn: saveProxy,
    gcTime: 0,
    onSuccess: async () => {
      setURL('')
      await onSaved()
      onClose()
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const valid = name.trim().length > 0 && (proxy || url.trim().length > 0)
    setInvalid(!valid)
    if (valid)
      save.mutate({ id: proxy?.id, name: name.trim(), url: url.trim() })
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) onClose()
      }}
    >
      <DialogContent
        showCloseButton={false}
        onInteractOutside={(event) => {
          if (save.isPending) event.preventDefault()
        }}
        onEscapeKeyDown={(event) => {
          if (save.isPending) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          restoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>{t(proxy ? 'editProxy' : 'addProxy')}</DialogTitle>
          <DialogDescription>{t('proxyFormHint')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5" noValidate>
          <div className="space-y-2">
            <label htmlFor="proxy-name" className="text-sm font-medium">
              {t('proxyName')}
            </label>
            <Input
              id="proxy-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={128}
              disabled={save.isPending}
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="proxy-url" className="text-sm font-medium">
              {t('proxyURL')}
            </label>
            <Input
              id="proxy-url"
              type="password"
              value={url}
              onChange={(event) => setURL(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              placeholder={proxy ? t('proxyKeepURL') : t('proxyURLExample')}
              disabled={save.isPending}
            />
            <p className="text-xs leading-5 text-muted-foreground">
              {t('proxyURLHint')}
            </p>
          </div>
          {invalid && (
            <p role="alert" className="text-sm text-error">
              {t('proxyInvalidInput')}
            </p>
          )}
          {save.isError && (
            <p role="alert" className="text-sm text-error">
              {t(proxyErrorKey(save.error))}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              onClick={onClose}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t('saveProxy')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
