import { useTranslation } from 'react-i18next'
import type { RequestRecord } from '@/lib/requests'

const strategyKeys = {
  none: 'requestRouteNone',
  affinity: 'requestRouteAffinity',
  parent: 'requestRouteParent',
  rotation: 'requestRouteRotation',
  quota_weighted: 'requestRouteWeighted',
} as const
const exclusionKeys = {
  model: 'requestRouteModel',
  policy: 'requestRoutePolicy',
  quota: 'requestRouteQuota',
  cooldown: 'requestRouteCooldown',
  concurrency: 'requestRouteConcurrency',
  unavailable: 'requestRouteUnavailable',
} as const

export function RouteDetails({ value }: { value: RequestRecord['route'] }) {
  const { t, i18n } = useTranslation()
  const numbers = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en')
  const exclusions = value
    ? Object.entries(exclusionKeys).filter(
        ([code]) => value.exclusions[code as keyof typeof exclusionKeys] > 0,
      )
    : []
  return (
    <section
      className="space-y-3 border-t border-border pt-4"
      aria-label={t('requestRouting')}
    >
      <h3 className="text-sm font-medium">{t('requestRouting')}</h3>
      {value ? (
        <>
          <p className="text-sm leading-6">{t(strategyKeys[value.strategy])}</p>
          {value.result === 'rejected' && value.strategy !== 'none' && (
            <p className="text-sm text-muted-foreground">
              {t('requestRouteRejected')}
            </p>
          )}
          {value.parent_fallback && (
            <p className="text-sm leading-6 text-muted-foreground">
              {t('requestRouteParentFallback')}
            </p>
          )}
          {value.candidates > 0 && (
            <p className="flex justify-between gap-4 text-sm text-muted-foreground">
              <span>{t('requestRouteCandidates')}</span>
              <span className="tabular-nums">
                {numbers.format(value.candidates)}
              </span>
            </p>
          )}
          {exclusions.length > 0 && (
            <div className="space-y-2">
              <h4 className="text-xs font-medium text-muted-foreground">
                {t('requestRouteExclusions')}
              </h4>
              <dl className="space-y-2 text-sm">
                {exclusions.map(([code, key]) => (
                  <div key={code} className="flex justify-between gap-4">
                    <dt className="text-muted-foreground">{t(key)}</dt>
                    <dd className="tabular-nums">
                      {numbers.format(
                        value.exclusions[code as keyof typeof exclusionKeys],
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
          <p className="text-xs leading-5 text-muted-foreground">
            {t('requestRoutingHint')}
          </p>
        </>
      ) : (
        <p className="text-sm leading-6 text-muted-foreground">
          {t('requestRoutingUnavailable')}
        </p>
      )}
    </section>
  )
}
