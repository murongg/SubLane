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
  const members = new Map<number, AllocationDetail['balances']>()
  for (const balance of detail.balances) {
    const windows = members.get(balance.user_id) ?? []
    windows.push(balance)
    members.set(balance.user_id, windows)
  }
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
          {[...members].map(([userID, windows]) => {
            const b = windows[0]
            const state = !detail.available
              ? 'allocationUnavailableShort'
              : windows.some((window) => window.admission === 'exhausted')
                ? 'allocationExhausted'
                : windows.some((window) => window.admission === 'risk_limited')
                  ? 'allocationRiskPaused'
                  : 'active'
            const oldestWindow =
              windows.find((window) => window.window_kind === '7d') ?? b
            return (
              <section key={userID} className="space-y-3 py-4">
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
                {windows.map((window, index) => {
                  const unit = window.mode === 'amount' ? 'USD' : 'M'
                  return (
                    <div
                      key={window.window_kind}
                      className={
                        index > 0
                          ? 'space-y-3 border-t border-border pt-3'
                          : 'space-y-3'
                      }
                    >
                      {windows.length > 1 && (
                        <div className="flex flex-wrap items-center justify-between gap-2 text-sm font-medium">
                          <h4>
                            {t(
                              window.window_kind === '5h'
                                ? 'allocationFiveHours'
                                : 'allocationSevenDays',
                            )}
                          </h4>
                          {window.admission !== 'active' && (
                            <span
                              className={
                                window.admission === 'exhausted'
                                  ? 'text-error'
                                  : 'text-warning'
                              }
                            >
                              {t(
                                window.admission === 'exhausted'
                                  ? 'allocationWindowReached'
                                  : 'allocationWindowRisk',
                              )}
                            </span>
                          )}
                        </div>
                      )}
                      <dl className="grid grid-cols-3 gap-3 text-sm">
                        <div>
                          <dt className="text-muted-foreground">
                            {t('allocationLimit')}
                          </dt>
                          <dd className="mt-1 tabular-nums">
                            {allocationValue(window.limit, window.mode)} {unit}
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground">
                            {t('allocationUsed')}
                          </dt>
                          <dd className="mt-1 tabular-nums">
                            {allocationValue(window.used, window.mode)} {unit}
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground">
                            {t('allocationRemaining')}
                          </dt>
                          <dd className="mt-1 tabular-nums">
                            {allocationValue(
                              Math.max(0, window.limit - window.used),
                              window.mode,
                            )}{' '}
                            {unit}
                          </dd>
                        </div>
                      </dl>
                      {(window.in_flight > 0 || window.pending_current > 0) && (
                        <div className="space-y-1 text-sm leading-6 text-muted-foreground">
                          <p>
                            {t('allocationRiskExposure', {
                              inFlight: window.in_flight,
                              pending: window.pending_current,
                              reserved: allocationValue(
                                window.reserved,
                                window.mode,
                              ),
                              unit,
                            })}
                          </p>
                          <p>
                            {t('allocationAdmissionRoom', {
                              room: allocationValue(
                                window.admission_room,
                                window.mode,
                              ),
                              unit,
                            })}
                          </p>
                        </div>
                      )}
                      {detail.available &&
                        window.admission === 'risk_limited' && (
                          <p className="text-sm leading-6 text-warning">
                            {t('allocationRiskLimit')}
                          </p>
                        )}
                      <p className="text-xs leading-5 text-muted-foreground">
                        {detail.config.period === 'dual'
                          ? timeZone
                          : detail.config.period === 'day'
                            ? t('allocationDailySchedule', {
                                time: detail.config.reset_time ?? '00:00',
                                zone: timeZone,
                              })
                            : t('allocationMonthlySchedule', {
                                day: detail.config.reset_day ?? 1,
                                time: detail.config.reset_time ?? '00:00',
                                zone: timeZone,
                              })}{' '}
                        ·{' '}
                        {t('allocationResetAt', {
                          date: date(window.reset_at),
                        })}
                        {window.mode === 'amount' &&
                          ` · ${t('allocationActualTokens', { tokens: allocationValue(window.tokens, 'tokens') })}`}
                      </p>
                    </div>
                  )
                })}
                {b.pending > oldestWindow.pending_current && (
                  <p className="text-sm leading-6 text-muted-foreground">
                    {t('allocationOlderPending', {
                      older: b.pending - oldestWindow.pending_current,
                    })}
                  </p>
                )}
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}
