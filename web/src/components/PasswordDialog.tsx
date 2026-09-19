import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { changePassword, replaceAuthState } from '@/lib/auth'
import { resetMemberPassword, type Member } from '@/lib/members'
import { validatePassword } from '@/lib/credentials'
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

const errorKeys = {
  current_password_invalid: 'currentPasswordInvalid',
  password_changed: 'passwordChangedRetry',
  rate_limited: 'passwordRateLimited',
  auth_busy: 'authBusy',
} as const

export function PasswordDialog({
  member,
  onClose,
  onSaved,
  returnFocus,
}: {
  member?: Pick<Member, 'id' | 'username'>
  onClose: () => void
  onSaved?: () => void
  returnFocus: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [error, setError] = useState<{
    field: string
    message: 'passwordHint' | 'passwordRequired' | 'passwordMismatch'
  } | null>(null)
  const mutation = useMutation({
    gcTime: 0,
    mutationFn: async (input: {
      current_password: string
      new_password: string
    }) => {
      if (member) {
        await resetMemberPassword(member.id, input.new_password)
        return
      }
      const state = await changePassword(input)
      await replaceAuthState(client, state)
    },
    onSuccess: () => {
      onSaved?.()
      onClose()
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const form = event.currentTarget
    const data = new FormData(form)
    const input = {
      current_password: String(data.get('current_password') ?? ''),
      new_password: String(data.get('new_password') ?? ''),
    }
    const validation = validatePassword(
      input.new_password,
      String(data.get('confirm') ?? ''),
    )
    const next =
      !member && !input.current_password
        ? { field: 'current_password', message: 'passwordRequired' as const }
        : validation
          ? {
              field:
                validation.field === 'password' ? 'new_password' : 'confirm',
              message: validation.message,
            }
          : null
    setError(next)
    mutation.reset()
    if (next) {
      const field = form.elements.namedItem(next.field)
      if (field instanceof HTMLInputElement) field.focus()
      return
    }
    mutation.mutate(input)
  }
  const fields = [
    ...(!member
      ? [
          {
            name: 'current_password',
            label: 'currentPassword',
            complete: 'current-password',
          } as const,
        ]
      : []),
    { name: 'new_password', label: 'newPassword', complete: 'new-password' },
    { name: 'confirm', label: 'confirmPassword', complete: 'new-password' },
  ] as const
  const code = mutation.error instanceof ApiError ? mutation.error.code : ''
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose()
      }}
    >
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        showCloseButton={!mutation.isPending}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          returnFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {member
              ? t('resetPasswordFor', { username: member.username })
              : t('changePassword')}
          </DialogTitle>
          <DialogDescription>{t('passwordSessionNotice')}</DialogDescription>
        </DialogHeader>
        <form noValidate onSubmit={submit} className="space-y-4">
          {fields.map((field) => (
            <div key={field.name} className="space-y-2">
              <label
                className="text-sm font-medium"
                htmlFor={'password-' + field.name}
              >
                {t(field.label)}
              </label>
              <Input
                id={'password-' + field.name}
                name={field.name}
                type="password"
                autoComplete={field.complete}
                maxLength={256}
                required
                disabled={mutation.isPending}
                aria-invalid={error?.field === field.name}
                aria-describedby={
                  error?.field === field.name
                    ? 'password-field-error'
                    : field.name === 'new_password'
                      ? 'password-length-hint'
                      : undefined
                }
              />
              {field.name === 'new_password' && (
                <p
                  id="password-length-hint"
                  className="text-xs text-muted-foreground"
                >
                  {t('passwordHint')}
                </p>
              )}
              {error?.field === field.name && (
                <p
                  id="password-field-error"
                  role="alert"
                  className="text-sm text-error"
                >
                  {t(error.message)}
                </p>
              )}
            </div>
          ))}
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                errorKeys[code as keyof typeof errorKeys] ??
                  'passwordSaveFailed',
              )}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={onClose}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <LoaderCircle
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              )}
              {t('savePassword')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
