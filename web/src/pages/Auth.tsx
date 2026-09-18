import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, LoaderCircle } from 'lucide-react'
import { authKey, setup, signIn } from '@/lib/auth'
import { ApiError } from '@/lib/request'
import { replaceAuthState } from '@/lib/query'
import { AuthShell } from '@/components/AuthShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

type FieldError = {
  field: string
  message:
    | 'usernameHint'
    | 'passwordHint'
    | 'passwordMismatch'
    | 'passwordRequired'
}
const messages = {
  invalid_credentials: 'invalidCredentials',
  rate_limited: 'authRateLimited',
  auth_busy: 'authBusy',
  origin_rejected: 'authOriginError',
  invalid_input: 'authInvalidInput',
  already_initialized: 'setupClosed',
} as const

export function Auth({ mode }: { mode: 'setup' | 'login' }) {
  const creating = mode === 'setup'
  const { t } = useTranslation()
  const client = useQueryClient()
  const navigate = useNavigate()
  const heading = useRef<HTMLHeadingElement>(null)
  const [fieldError, setFieldError] = useState<FieldError | null>(null)
  useEffect(() => {
    if (creating) heading.current?.focus()
  }, [creating])
  const mutation = useMutation({
    // Drop submitted credentials when the authentication form is no longer observed.
    gcTime: 0,
    mutationFn: creating ? setup : signIn,
    onSuccess: (state) => replaceAuthState(client, state),
    onError: (error) => {
      if (error instanceof ApiError && error.code === 'already_initialized')
        void client.invalidateQueries({ queryKey: authKey })
    },
  })
  const validate = (
    field: string,
    message: FieldError['message'],
    form: HTMLFormElement,
  ) => {
    setFieldError({ field, message })
    const input = form.elements.namedItem(field)
    if (input instanceof HTMLInputElement) input.focus()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const form = event.currentTarget
    const data = new FormData(form)
    const input = {
      username: String(data.get('username') ?? '').trim(),
      password: String(data.get('password') ?? ''),
    }
    setFieldError(null)
    mutation.reset()
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$/.test(input.username))
      return validate('username', 'usernameHint', form)
    // Match the backend's Unicode character count instead of counting UTF-16 code units.
    const length = Array.from(input.password).length
    if (!length) return validate('password', 'passwordRequired', form)
    if (creating && (length < 8 || length > 20))
      return validate('password', 'passwordHint', form)
    if (creating && input.password !== data.get('confirm'))
      return validate('confirm', 'passwordMismatch', form)
    mutation.mutate(input)
  }
  const errorCode =
    mutation.error instanceof ApiError ? mutation.error.code : 'unavailable'
  const errorKey = Object.hasOwn(messages, errorCode)
    ? messages[errorCode as keyof typeof messages]
    : 'unavailable'
  const field = (
    name: string,
    label: 'username' | 'password' | 'confirmPassword',
    autoComplete: string,
    hint?: 'usernameHint' | 'passwordHint',
  ) => (
    <div className="space-y-2">
      <label htmlFor={'auth-' + name} className="text-sm font-medium">
        {t(label)}
      </label>
      <Input
        id={'auth-' + name}
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
            ? 'field-error'
            : hint
              ? 'hint-' + name
              : undefined
        }
      />
      {fieldError?.field === name ? (
        <p id="field-error" role="alert" className="text-sm text-error">
          {t(fieldError.message)}
        </p>
      ) : hint ? (
        <p
          id={'hint-' + name}
          className="text-xs leading-5 text-muted-foreground"
        >
          {t(hint)}
        </p>
      ) : null}
    </div>
  )
  return (
    <AuthShell>
      {creating && (
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2.5 mb-6 text-muted-foreground"
          disabled={mutation.isPending}
          onClick={() => void navigate({ to: '/setup' })}
        >
          <ArrowLeft aria-hidden="true" />
          {t('backToWelcome')}
        </Button>
      )}
      <h1
        ref={heading}
        tabIndex={-1}
        className="text-2xl font-semibold tracking-tight outline-none"
      >
        {t(creating ? 'createAdministrator' : 'signInTitle')}
      </h1>
      <p className="mb-7 mt-2 text-sm leading-6 text-muted-foreground">
        {t(creating ? 'setupDescription' : 'signInDescription')}
      </p>
      <form noValidate onSubmit={submit} className="space-y-5">
        {field(
          'username',
          'username',
          'username',
          creating ? 'usernameHint' : undefined,
        )}
        {field(
          'password',
          'password',
          creating ? 'new-password' : 'current-password',
          creating ? 'passwordHint' : undefined,
        )}
        {creating && field('confirm', 'confirmPassword', 'new-password')}
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
        <Button type="submit" className="w-full" disabled={mutation.isPending}>
          {mutation.isPending && (
            <LoaderCircle
              className="motion-safe:animate-spin"
              aria-hidden="true"
            />
          )}
          {t(
            mutation.isPending
              ? creating
                ? 'creatingAdministrator'
                : 'signingIn'
              : creating
                ? 'createAdministrator'
                : 'signIn',
          )}
        </Button>
      </form>
    </AuthShell>
  )
}
