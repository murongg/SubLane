import { useId, useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  budgetErrorKey,
  parseBudgetMillions,
  settleBudget,
  type BudgetPending,
} from '@/lib/budgets'
import { Button } from './ui/Button'
import { Input } from './ui/Input'

export function BudgetSettlement({
  userID,
  pending,
  onSaved,
  onBusy,
}: {
  userID: number
  pending: BudgetPending
  onSaved: () => Promise<unknown>
  onBusy: (busy: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: settleBudget,
    onSuccess: onSaved,
    onSettled: () => onBusy(false),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (mutation.isPending) return
    const value = new FormData(event.currentTarget).get('tokens')
    const tokens = parseBudgetMillions(String(value ?? ''))
    const bad =
      tokens === null || tokens < pending.known_tokens || tokens > 2_000_000_000
    setInvalid(bad)
    if (bad || tokens === null) return
    onBusy(true)
    mutation.mutate({ userID, request_id: pending.request_id, tokens })
  }
  return (
    <form className="space-y-2 border-t border-border py-3" onSubmit={submit}>
      <p className="break-all text-xs font-medium">{pending.request_id}</p>
      <p className="text-xs text-muted-foreground">
        {new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
          dateStyle: 'medium',
          timeStyle: 'short',
          timeZone: 'UTC',
        }).format(pending.started_at * 1000)}{' '}
        UTC ·{' '}
        {t('budgetKnownTokens', {
          value: new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en', {
            maximumFractionDigits: 6,
          }).format(pending.known_tokens / 1_000_000),
        })}
      </p>
      <label htmlFor={id} className="block text-sm">
        {t('budgetSettleTotal', { request: pending.request_id })}
      </label>
      <div className="flex flex-wrap gap-2">
        <Input
          className="min-w-0 flex-1 basis-36"
          id={id}
          name="tokens"
          type="number"
          required
          min={pending.known_tokens / 1_000_000}
          max={2000}
          step={0.000001}
          disabled={mutation.isPending}
        />
        <Button variant="outline" disabled={mutation.isPending}>
          {t('budgetSettle')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('budgetUnitHint')}</p>
      {invalid && (
        <p role="alert" className="text-sm text-error">
          {t('budgetSettlementInvalid')}
        </p>
      )}
      {mutation.isError && (
        <p role="alert" className="text-sm text-error">
          {t(budgetErrorKey(mutation.error))}
        </p>
      )}
    </form>
  )
}
