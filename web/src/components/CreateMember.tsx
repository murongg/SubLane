import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { createMember, type Member } from '@/lib/members'
import { validateCredentials, type CredentialError } from '@/lib/credentials'
import { ApiError } from '@/lib/request'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from './ui/Sheet'

export function CreateMember({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: (member: Member) => void
}) {
  const { t } = useTranslation()
  const [fieldError, setFieldError] = useState<CredentialError | null>(null)
  const mutation = useMutation({
    // The parent unmounts this form on close, discarding credentials from the mutation cache.
    gcTime: 0,
    mutationFn: createMember,
    onSuccess: onCreated,
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const form = event.currentTarget
    const data = new FormData(form)
    const input = {
      username: String(data.get('username') ?? '').trim(),
      password: String(data.get('password') ?? ''),
    }
    const error = validateCredentials(input, String(data.get('confirm') ?? ''))
    setFieldError(error)
    mutation.reset()
    if (error) {
      const field = form.elements.namedItem(error.field)
      if (field instanceof HTMLInputElement) field.focus()
      return
    }
    mutation.mutate(input)
  }
  const fields = [
    {
      name: 'username',
      label: 'username',
      hint: 'usernameHint',
      autoComplete: 'off',
    },
    {
      name: 'password',
      label: 'password',
      hint: 'passwordHint',
      autoComplete: 'new-password',
    },
    {
      name: 'confirm',
      label: 'confirmPassword',
      hint: null,
      autoComplete: 'new-password',
    },
  ] as const
  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose()
      }}
    >
      <SheetContent
        className="w-full overflow-y-auto sm:max-w-md"
        closeDisabled={mutation.isPending}
      >
        <SheetHeader>
          <SheetTitle>{t('addMember')}</SheetTitle>
          <SheetDescription>{t('addMemberDescription')}</SheetDescription>
        </SheetHeader>
        <form noValidate onSubmit={submit} className="space-y-5 px-4 pb-6">
          {fields.map(({ name, label, hint, autoComplete }) => (
            <div key={name} className="space-y-2">
              <label htmlFor={'member-' + name} className="text-sm font-medium">
                {t(label)}
              </label>
              <Input
                id={'member-' + name}
                name={name}
                type={name === 'username' ? 'text' : 'password'}
                autoComplete={autoComplete}
                autoCapitalize="none"
                spellCheck={false}
                maxLength={name === 'username' ? 32 : 256}
                required
                disabled={mutation.isPending}
                aria-invalid={fieldError?.field === name}
                aria-describedby={
                  fieldError?.field === name
                    ? 'member-field-error'
                    : hint
                      ? 'member-hint-' + name
                      : undefined
                }
              />
              {fieldError?.field === name ? (
                <p
                  id="member-field-error"
                  role="alert"
                  className="text-sm text-error"
                >
                  {t(fieldError.message)}
                </p>
              ) : (
                hint && (
                  <p
                    id={'member-hint-' + name}
                    className="text-xs leading-5 text-muted-foreground"
                  >
                    {t(hint)}
                  </p>
                )
              )}
            </div>
          ))}
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                mutation.error instanceof ApiError &&
                  mutation.error.code === 'username_taken'
                  ? 'memberUsernameTaken'
                  : mutation.error instanceof ApiError &&
                      mutation.error.code === 'auth_busy'
                    ? 'authBusy'
                    : 'memberCreateFailed',
              )}
            </p>
          )}
          <div className="flex flex-wrap justify-end gap-2 pt-3">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
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
              {t(mutation.isPending ? 'creatingMember' : 'createMember')}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  )
}
