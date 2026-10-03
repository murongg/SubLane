import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LoaderCircle } from 'lucide-react'
import { accountErrorKey, deleteAccount, type Account } from '@/lib/accounts'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function DeleteAccount({
  account,
  onClose,
  onDeleted,
  restoreFocus,
  remove = deleteAccount,
  titleKey = 'deleteAccountTitle',
  descriptionKey = 'deleteAccountDescription',
  actionKey = 'deleteAccount',
}: {
  account: Pick<Account, 'id' | 'name'>
  remove?: (id: string) => Promise<unknown>
  titleKey?: 'deleteAccountTitle' | 'deleteChannelTitle'
  descriptionKey?: 'deleteAccountDescription' | 'deleteChannelDescription'
  actionKey?: 'deleteAccount' | 'deleteChannel'
  onClose: () => void
  onDeleted: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const mutation = useMutation({
    mutationFn: () => remove(account.id),
    onSuccess: async () => {
      await onDeleted()
      onClose()
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose()
      }}
    >
      <DialogContent
        className="[@media(pointer:coarse)]:[&_button]:min-h-11"
        showCloseButton={false}
        onInteractOutside={(event) => {
          if (mutation.isPending) event.preventDefault()
        }}
        onEscapeKeyDown={(event) => {
          if (mutation.isPending) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          restoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>{t(titleKey)}</DialogTitle>
          <DialogDescription>
            {t(descriptionKey, { name: account.name })}
          </DialogDescription>
        </DialogHeader>
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t(accountErrorKey(mutation.error))}
          </p>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
          >
            {mutation.isPending && (
              <LoaderCircle
                className="motion-safe:animate-spin"
                aria-hidden="true"
              />
            )}
            {t(actionKey)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
