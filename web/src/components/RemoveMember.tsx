import { useMutation } from '@tanstack/react-query'
import { useRef } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { removeMember, type Member } from '@/lib/members'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function RemoveMember({
  member,
  onClose,
  onRemoved,
  restoreFocus,
}: {
  member: Member
  onClose: () => void
  onRemoved: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const cancelButton = useRef<HTMLButtonElement>(null)
  const mutation = useMutation({
    mutationFn: () => removeMember(member.id),
    onSuccess: async () => {
      onClose()
      await onRemoved()
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
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          cancelButton.current?.focus()
        }}
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
          <DialogTitle>{t('memberRemoveTitle')}</DialogTitle>
          <DialogDescription>
            {t('memberRemoveDescription', { username: member.username })}
          </DialogDescription>
        </DialogHeader>
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t('memberRemoveFailed')}
          </p>
        )}
        <DialogFooter>
          <Button
            ref={cancelButton}
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
            {t('memberRemoveConfirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
