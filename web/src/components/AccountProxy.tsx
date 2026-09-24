import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { accountErrorKey, bindAccountProxy, type Account } from '@/lib/accounts'
import { proxyOptions } from '@/lib/proxies'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function AccountProxy({
  account,
  onClose,
  onChanged,
  restoreFocus,
}: {
  account: Account
  onClose: () => void
  onChanged: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const [proxyID, setProxyID] = useState(account.proxy_id)
  const query = useQuery(proxyOptions)
  const save = useMutation({
    mutationFn: bindAccountProxy,
    onSuccess: async () => {
      await onChanged()
      onClose()
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!query.data || save.isPending) return
    save.mutate({ id: account.id, proxy_id: proxyID })
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
          <DialogTitle>{t('accountProxy')}</DialogTitle>
          <DialogDescription>
            {account.name} · {t('accountProxyHint')}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5">
          {query.isPending ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('loadingProxies')}
            </p>
          ) : query.isError ? (
            <div role="alert" className="space-y-2 text-sm text-error">
              <p>{t('proxiesLoadFailed')}</p>
              <Button
                type="button"
                variant="outline"
                onClick={() => query.refetch()}
              >
                {t('reconnect')}
              </Button>
            </div>
          ) : (
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">
                {t('accountProxy')}
              </legend>
              {[
                { id: '', name: t('directConnection') },
                ...query.data.proxies,
              ].map((proxy) => (
                <label
                  key={proxy.id}
                  className="flex min-h-10 items-center gap-3 rounded-md border border-border px-3 py-2 text-sm has-[:checked]:border-foreground has-[:checked]:bg-muted"
                >
                  <input
                    type="radio"
                    name="proxy"
                    value={proxy.id}
                    checked={proxyID === proxy.id}
                    onChange={() => setProxyID(proxy.id)}
                    disabled={save.isPending}
                    className="accent-foreground"
                  />
                  <span className="min-w-0 break-words">{proxy.name}</span>
                </label>
              ))}
            </fieldset>
          )}
          {save.isError && (
            <p role="alert" className="text-sm text-error">
              {t(accountErrorKey(save.error))}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={save.isPending}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={!query.data || save.isPending}>
              {save.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t('saveProxyBinding')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
