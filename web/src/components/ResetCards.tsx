import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { resetCardsOptions } from '@/lib/resets'
import { Button } from './ui/Button'
import { Status } from './Status'

const labels = {
  available: 'resetCardAvailable',
  redeeming: 'resetCardRedeeming',
  redeemed: 'resetCardRedeemed',
  expired: 'resetCardExpired',
  unknown: 'resetCardUnknown',
} as const

export function ResetCards({
  id,
  enabled,
  timeZone,
  pending,
}: {
  id: string
  enabled: boolean
  timeZone: string
  pending: boolean
}) {
  const { t, i18n } = useTranslation()
  const title = useId()
  const query = useQuery({ ...resetCardsOptions(id), enabled })
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone,
  })
  return (
    <section
      className="space-y-3 border-b border-border pb-5"
      aria-labelledby={title}
    >
      <div className="flex items-center justify-between gap-3">
        <h3 id={title} className="text-sm font-medium">
          {t('resetCardsTitle')}
        </h3>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={!enabled || query.isFetching}
          onClick={() => query.refetch()}
        >
          <RefreshCw
            aria-hidden="true"
            className={query.isFetching ? 'motion-safe:animate-spin' : ''}
          />
          {t('resetCardsRefresh')}
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        {t('resetCardsPolicy')}
      </p>
      {!enabled ? (
        <p className="text-sm text-muted-foreground">
          {t('resetCardsDisabled')}
        </p>
      ) : (
        <>
          {query.isPending && (
            <p role="status" className="text-sm text-muted-foreground">
              {t('resetCardsLoading')}
            </p>
          )}
          {query.isError && (
            <p role="alert" className="text-sm text-warning">
              {t('resetCardsLoadFailed')}
            </p>
          )}
          {query.data && (
            <>
              <p className="text-xs text-muted-foreground">
                {t('resetCardsAvailable', {
                  count:
                    query.data.available_count ?? t('usageResetCreditsUnknown'),
                })}
              </p>
              {query.data.cards.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('resetCardsEmpty')}
                </p>
              ) : (
                <ul
                  className="max-h-64 space-y-0 divide-y divide-border overflow-y-auto pe-1"
                  aria-label={t('resetCardsTitle')}
                  tabIndex={0}
                >
                  {query.data.cards.map((card) => (
                    <li
                      key={card.id}
                      className="space-y-2 py-3 first:pt-0 last:pb-0"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="min-w-0 break-words text-sm font-medium">
                          {card.title || t('resetCardDefaultTitle')}
                        </span>
                        {!pending && card.id === query.data.next_card_id && (
                          <Status kind="info">{t('resetCardNext')}</Status>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Status
                          kind={
                            card.supported && card.status === 'available'
                              ? 'success'
                              : 'neutral'
                          }
                        >
                          {t(
                            card.supported
                              ? labels[card.status]
                              : 'resetCardUnsupported',
                          )}
                        </Status>
                        <span className="text-xs text-muted-foreground">
                          {card.expires_at === null
                            ? t('resetCardNoExpiry')
                            : t('resetCardExpires', {
                                date: dates.format(card.expires_at * 1000),
                              })}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {t('resetCardGranted', {
                          date: dates.format(card.granted_at * 1000),
                        })}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
              <p className="text-xs text-muted-foreground">
                {t('resetCardsUpdated', {
                  date: dates.format(query.data.observed_at * 1000),
                })}
              </p>
            </>
          )}
        </>
      )}
    </section>
  )
}
