import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  saveWorkspaceMember,
  setWorkspaceMemberRole,
  type Member,
} from '@/lib/members'
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

export function MembershipEditor({
  member,
  onClose,
  onSaved,
}: {
  member?: Member
  onClose: () => void
  onSaved: (member: Member) => Promise<void>
}) {
  const { t } = useTranslation()
  const [username, setUsername] = useState(member?.username ?? '')
  const [role, setRole] = useState<'member' | 'admin'>(member?.role ?? 'member')
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: (input: { username: string; role: 'member' | 'admin' }) =>
      member
        ? setWorkspaceMemberRole(member.id, input.role)
        : saveWorkspaceMember(input),
    onSuccess: onSaved,
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const name = username.trim()
    setInvalid(!name)
    if (!name) return
    mutation.mutate({ username: name, role })
  }
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
          <SheetTitle>
            {t(member ? 'memberRoleChange' : 'memberExistingAdd')}
          </SheetTitle>
          <SheetDescription>
            {t(
              member
                ? 'memberRoleChangeDescription'
                : 'memberExistingDescription',
              { username: member?.username },
            )}
          </SheetDescription>
        </SheetHeader>
        <form onSubmit={submit} className="space-y-5 px-4 pb-6" noValidate>
          <div className="space-y-2">
            <label
              htmlFor="existing-member-name"
              className="text-sm font-medium"
            >
              {t('username')}
            </label>
            <Input
              id="existing-member-name"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              readOnly={!!member}
              required
              maxLength={32}
              autoComplete="off"
              disabled={mutation.isPending}
              aria-invalid={invalid}
            />
            {invalid && (
              <p role="alert" className="text-sm text-error">
                {t('memberExistingNameRequired')}
              </p>
            )}
          </div>
          <fieldset disabled={mutation.isPending} className="space-y-3">
            <legend className="text-sm font-medium">{t('role')}</legend>
            {(['member', 'admin'] as const).map((choice) => (
              <label
                key={choice}
                className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 has-disabled:cursor-not-allowed has-disabled:opacity-50"
              >
                <input
                  type="radio"
                  name="workspace-role"
                  checked={role === choice}
                  onChange={() => setRole(choice)}
                  disabled={!!member && !member.enabled && choice === 'admin'}
                  aria-label={t(
                    choice === 'admin' ? 'administrator' : 'member',
                  )}
                  aria-describedby={`workspace-role-${choice}-hint`}
                  className="mt-0.5 size-4 accent-primary"
                />
                <span className="space-y-1">
                  <span className="block text-sm font-medium">
                    {t(choice === 'admin' ? 'administrator' : 'member')}
                  </span>
                  <span
                    id={`workspace-role-${choice}-hint`}
                    className="block text-xs leading-5 text-muted-foreground"
                  >
                    {t(
                      choice === 'admin'
                        ? 'memberAdminRoleHint'
                        : 'memberBasicRoleHint',
                    )}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                mutation.error instanceof ApiError
                  ? mutation.error.code === 'member_already_exists'
                    ? 'memberExistingAlreadyHere'
                    : mutation.error.code === 'invalid_input'
                      ? member
                        ? 'memberRoleInvalid'
                        : 'memberExistingNotFound'
                      : 'memberExistingSaveFailed'
                  : 'memberExistingSaveFailed',
              )}
            </p>
          )}
          <div className="flex justify-end gap-2 pt-3">
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
              {t('saveGroupAccess')}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  )
}
