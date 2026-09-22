import { useTranslation } from 'react-i18next'
import { type AllocationDetail, allocationValue } from '@/lib/allocations'
import { Status } from './Status'
export function AllocationBalances({ detail }: { detail: AllocationDetail }) {
  const { t, i18n } = useTranslation()
  const date = (n: number) =>
    new Date(n * 1000).toLocaleString(i18n.resolvedLanguage ?? 'en')
  return (
    <div className="space-y-4">
      {detail.config.mode === 'ratio' && (
        <p className="text-sm leading-6 text-muted-foreground">
          {t('allocationEstimateHint')}
        </p>
      )}
      {!detail.available && (
        <p className="text-sm text-muted-foreground">
          {t('allocationUnavailable')}
        </p>
      )}
      {detail.balances.length === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">
          {t('allocationNoBalances')}
        </p>
      ) : (
        <div className="divide-y divide-border">
          {detail.balances.map((b) => {
            const unit =
              b.mode === 'ratio'
                ? t('allocationPoints')
                : b.mode === 'amount'
                  ? 'USD'
                  : 'M'
            return (
              <section
                key={`${b.user_id}-${b.window_id}`}
                className="space-y-3 py-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <h3 className="break-words text-sm font-medium">
                    {b.username}
                    {b.window_kind &&
                      ` · ${b.window_kind === 'primary' ? t('allocationPrimary') : t('allocationSecondary')}`}
                    {b.account_id && (
                      <span className="ml-2 text-xs text-muted-foreground">
                        {b.account_id.slice(0, 8)}
                      </span>
                    )}
                  </h3>
                  <Status
                    kind={
                      !detail.available
                        ? 'neutral'
                        : b.pending > 0
                          ? 'warning'
                          : b.used >= b.limit
                            ? 'error'
                            : 'success'
                    }
                  >
                    {t(
                      !detail.available
                        ? 'allocationUnavailableShort'
                        : b.pending > 0
                          ? 'allocationPending'
                          : b.used >= b.limit
                            ? 'allocationExhausted'
                            : 'active',
                    )}
                  </Status>
                </div>
                <dl className="grid grid-cols-3 gap-3 text-sm">
                  <div>
                    <dt className="text-muted-foreground">
                      {t('allocationLimit')}
                    </dt>
                    <dd className="mt-1 tabular-nums">
                      {allocationValue(b.limit, b.mode)} {unit}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">
                      {t('allocationUsed')}
                    </dt>
                    <dd className="mt-1 tabular-nums">
                      {allocationValue(b.used, b.mode)} {unit}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">
                      {t('allocationRemaining')}
                    </dt>
                    <dd className="mt-1 tabular-nums">
                      {allocationValue(Math.max(0, b.limit - b.used), b.mode)}{' '}
                      {unit}
                    </dd>
                  </div>
                </dl>
                <p className="text-xs leading-5 text-muted-foreground">
                  {detail.config.period !== 'upstream' &&
                    `${t(detail.config.period === 'day' ? 'budgetDaily' : 'budgetMonthly')} · `}
                  {t('allocationResetAt', { date: date(b.reset_at) })}
                  {b.mode !== 'tokens' &&
                    ` · ${t('allocationActualTokens', { tokens: allocationValue(b.tokens, 'tokens') })}`}
                </p>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}
