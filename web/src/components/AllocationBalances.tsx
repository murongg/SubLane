import { useTranslation } from 'react-i18next'
import { type AllocationDetail, allocationValue } from '@/lib/allocations'
import { Status } from './Status'
import { formatInstanceDate, useTimeZone } from '@/lib/timezone'

export function AllocationBalances({ detail }: { detail: AllocationDetail }) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const date = (n: number) =>
    formatInstanceDate(n * 1000, i18n.resolvedLanguage ?? 'en', timeZone, {
      dateStyle: 'short',
      timeStyle: 'medium',
    })
  return (
    <div className="space-y-4">
      {detail.config.mode === 'ratio' && (
        <p className="text-sm leading-6 text-muted-foreground">
          {t(
            detail.config.ratio_unit === 'amount'
              ? 'allocationRatioAmountBalanceHint'
              : 'allocationRatioTokensBalanceHint',
          )}
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
            const state = !detail.available
              ? 'allocationUnavailableShort'
              : b.admission === 'exhausted'
                ? 'allocationExhausted'
                : b.admission === 'risk_limited'
                  ? 'allocationRiskPaused'
                  : 'active'
            const unit = b.mode === 'amount' ? 'USD' : 'M'
            return (
              <section key={b.user_id} className="space-y-3 py-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <h3 className="break-words text-sm font-medium">
                    {b.username}
                  </h3>
                  <Status
                    kind={
                      state === 'allocationUnavailableShort'
                        ? 'neutral'
                        : state === 'allocationRiskPaused'
                          ? 'warning'
                          : state === 'allocationExhausted'
                            ? 'error'
                            : 'success'
                    }
                  >
                    {t(state)}
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
                {(b.in_flight > 0 || b.pending_current > 0) && (
                  <div className="space-y-1 text-sm leading-6 text-muted-foreground">
                    <p>
                      {t('allocationRiskExposure', {
                        inFlight: b.in_flight,
                        pending: b.pending_current,
                        reserved: allocationValue(b.reserved, b.mode),
                        unit,
                      })}
                    </p>
                    <p>
                      {t('allocationAdmissionRoom', {
                        room: allocationValue(b.admission_room, b.mode),
                        unit,
                      })}
                    </p>
                  </div>
                )}
                {detail.available && b.admission === 'risk_limited' && (
                  <p className="text-sm leading-6 text-warning">
                    {t('allocationRiskLimit')}
                  </p>
                )}
                {b.pending > b.pending_current && (
                  <p className="text-sm leading-6 text-muted-foreground">
                    {t('allocationOlderPending', {
                      older: b.pending - b.pending_current,
                    })}
                  </p>
                )}
                <p className="text-xs leading-5 text-muted-foreground">
                  {detail.config.period === 'day'
                    ? t('allocationDailySchedule', {
                        time: detail.config.reset_time ?? '00:00',
                        zone: timeZone,
                      })
                    : t('allocationMonthlySchedule', {
                        day: detail.config.reset_day ?? 1,
                        time: detail.config.reset_time ?? '00:00',
                        zone: timeZone,
                      })}{' '}
                  · {t('allocationResetAt', { date: date(b.reset_at) })}
                  {b.mode === 'amount' &&
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
