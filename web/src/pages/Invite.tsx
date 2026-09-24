import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { registerInvitation } from '@/lib/auth'
import { replaceAuthState } from '@/lib/query'
import { ApiError } from '@/lib/request'
import { selectWorkspace } from '@/lib/workspace'
import { validateCredentials, type CredentialError } from '@/lib/credentials'
import { AuthShell } from '@/components/AuthShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

export function Invite() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const href = useRouterState({ select: (state) => state.location.href })
  const url = new URL(href, window.location.origin)
  const workspace = Number(url.searchParams.get('workspace'))
  const token = url.hash.slice(1)
  const validLink =
    Number.isSafeInteger(workspace) &&
    workspace > 0 &&
    /^[A-Za-z0-9_-]{43}$/.test(token)
  const heading = useRef<HTMLHeadingElement>(null)
  const [fieldError, setFieldError] = useState<CredentialError | null>(null)
  useEffect(() => heading.current?.focus(), [])
  const mutation = useMutation({
    gcTime: 0,
    mutationFn: (input: { username: string; password: string }) =>
      registerInvitation({ ...input, token, workspace }),
    onSuccess: async (state) => {
      selectWorkspace(workspace)
      await replaceAuthState(client, state)
    },
  })

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending || !validLink) return
    const form = event.currentTarget
    const data = new FormData(form)
    const input = {
      username: String(data.get('username') ?? '').trim(),
      password: String(data.get('password') ?? ''),
    }
    setFieldError(null)
    mutation.reset()
    const validation = validateCredentials(
      input,
      String(data.get('confirm') ?? ''),
    )
    if (validation) {
      setFieldError(validation)
      const field = form.elements.namedItem(validation.field)
      if (field instanceof HTMLInputElement) field.focus()
      return
    }
    mutation.mutate(input)
  }

  const errorCode =
    mutation.error instanceof ApiError ? mutation.error.code : 'unavailable'
  const errorKey =
    errorCode === 'invitation_invalid'
      ? 'inviteInvalid'
      : errorCode === 'username_taken'
        ? 'memberUsernameTaken'
        : errorCode === 'rate_limited'
          ? 'authRateLimited'
          : errorCode === 'auth_busy'
            ? 'authBusy'
            : errorCode === 'origin_rejected'
              ? 'authOriginError'
              : 'inviteRegisterFailed'

  return (
    <AuthShell>
      <h1
        ref={heading}
        tabIndex={-1}
        className="text-2xl font-semibold tracking-tight outline-none"
      >
        {t('inviteJoinTitle')}
      </h1>
      <p className="mb-7 mt-2 text-sm leading-6 text-muted-foreground">
        {t('inviteJoinDescription')}
      </p>
      {!validLink ? (
        <p role="alert" className="text-sm text-error">
          {t('inviteInvalid')}
        </p>
      ) : (
        <form noValidate onSubmit={submit} className="space-y-5">
          {(
            [
              ['username', 'username', 'username', 'usernameHint'],
              ['password', 'password', 'new-password', 'passwordHint'],
              ['confirm', 'confirmPassword', 'new-password', null],
            ] as const
          ).map(([name, label, autoComplete, hint]) => (
            <div className="space-y-2" key={name}>
              <label htmlFor={`invite-${name}`} className="text-sm font-medium">
                {t(label)}
              </label>
              <Input
                id={`invite-${name}`}
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
                    ? `invite-error-${name}`
                    : hint
                      ? `invite-hint-${name}`
                      : undefined
                }
              />
              {fieldError?.field === name ? (
                <p
                  id={`invite-error-${name}`}
                  role="alert"
                  className="text-sm text-error"
                >
                  {t(fieldError.message)}
                </p>
              ) : hint ? (
                <p
                  id={`invite-hint-${name}`}
                  className="text-xs leading-5 text-muted-foreground"
                >
                  {t(hint)}
                </p>
              ) : null}
            </div>
          ))}
          {mutation.isError && (
            <p role="alert" className="text-sm leading-6 text-error">
              {t(errorKey, {
                seconds:
                  mutation.error instanceof ApiError
                    ? mutation.error.retryAfter || 60
                    : 60,
              })}
            </p>
          )}
          <Button
            type="submit"
            className="w-full"
            disabled={mutation.isPending}
          >
            {mutation.isPending && (
              <LoaderCircle
                className="motion-safe:animate-spin"
                aria-hidden="true"
              />
            )}
            {t(
              mutation.isPending ? 'inviteRegistering' : 'inviteCreateAccount',
            )}
          </Button>
        </form>
      )}
      <p className="mt-6 text-sm text-muted-foreground">
        <Link
          to="/login"
          className="underline underline-offset-4 hover:text-foreground"
        >
          {t('inviteBackToLogin')}
        </Link>
      </p>
    </AuthShell>
  )
}
