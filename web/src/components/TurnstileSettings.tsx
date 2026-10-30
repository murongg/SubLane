import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { authKey } from '@/lib/auth'
import { ApiError } from '@/lib/request'
import { saveTurnstile, turnstileOptions } from '@/lib/turnstile'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { Button } from './ui/Button'
import { Input } from './ui/Input'

export function TurnstileSettings({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const options = turnstileOptions(userID)
  const query = useQuery(options)
  const [saved, setSaved] = useState(false)
  const [secret, setSecret] = useState('')
  const mutation = useAdminMutation({
    userID,
    mutationFn: saveTurnstile,
    onSuccess: async (result) => {
      client.setQueryData(options.queryKey, result)
      setSecret('')
      setSaved(true)
      await client.invalidateQueries({ queryKey: authKey })
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const data = new FormData(event.currentTarget)
    setSaved(false)
    mutation.mutate({
      enabled: data.get('enabled') === 'on',
      site_key: String(data.get('site_key') ?? '').trim(),
      ...(secret ? { secret_key: secret.trim() } : {}),
    })
  }
  return (
    <section
      className="mt-10 border-t border-border pt-6"
      aria-labelledby="turnstile-title"
    >
      <h2 id="turnstile-title" className="font-medium">
        {t('turnstileTitle')}
      </h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t('turnstileDescription')}
      </p>
      {query.isPending ? (
        <p role="status" className="mt-6 text-sm text-muted-foreground">
          {t('loading')}
        </p>
      ) : query.isError ? (
        <div className="mt-6 space-y-3">
          <p role="alert" className="text-sm text-error">
            {t('turnstileSettingsFailed')}
          </p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('turnstileRetry')}
          </Button>
        </div>
      ) : (
        <form
          key={`${query.data.enabled}-${query.data.site_key}`}
          className="mt-6 max-w-md space-y-4"
          onSubmit={submit}
          onChange={() => {
            setSaved(false)
            mutation.reset()
          }}
        >
          <label className="flex items-center gap-2 text-sm font-medium">
            <input
              type="checkbox"
              name="enabled"
              defaultChecked={query.data.enabled}
              disabled={mutation.isPending}
              className="size-4 accent-primary"
            />
            {t('turnstileEnabled')}
          </label>
          <div className="space-y-2">
            <label htmlFor="turnstile-site-key" className="text-sm font-medium">
              {t('turnstileSiteKey')}
            </label>
            <Input
              id="turnstile-site-key"
              name="site_key"
              defaultValue={query.data.site_key}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
              disabled={mutation.isPending}
            />
          </div>
          <div className="space-y-2">
            <label
              htmlFor="turnstile-secret-key"
              className="text-sm font-medium"
            >
              {t('turnstileSecretKey')}
            </label>
            <Input
              id="turnstile-secret-key"
              type="password"
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
              maxLength={256}
              autoComplete="new-password"
              spellCheck={false}
              disabled={mutation.isPending}
              aria-describedby="turnstile-secret-hint"
            />
            <p
              id="turnstile-secret-hint"
              className="text-xs leading-5 text-muted-foreground"
            >
              {t(
                query.data.secret_configured
                  ? 'turnstileSecretSaved'
                  : 'turnstileSecretHint',
              )}
            </p>
          </div>
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(
                mutation.error instanceof ApiError &&
                  mutation.error.code === 'invalid_turnstile_settings'
                  ? 'turnstileSettingsInvalid'
                  : 'turnstileSettingsFailed',
              )}
            </p>
          )}
          {saved && (
            <p role="status" className="text-sm text-success">
              {t('turnstileSaved')}
            </p>
          )}
          <Button type="submit" disabled={mutation.isPending}>
            {t('turnstileSave')}
          </Button>
        </form>
      )}
    </section>
  )
}
