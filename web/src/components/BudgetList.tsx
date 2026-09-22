import { useTranslation } from 'react-i18next'
import type { Budget } from '@/lib/budgets'
import { Button } from './ui/Button'
import { Status } from './Status'

export function BudgetList({
  rules,
  onEdit,
}: {
  rules: Budget[]
  onEdit?: (budget: Budget) => void
}) {
  const { t, i18n } = useTranslation()
  const number = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en', {
    maximumFractionDigits: 6,
  })
  const date = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'UTC',
  })
  if (!rules.length)
    return (
      <p className="py-4 text-sm text-muted-foreground">{t('budgetsEmpty')}</p>
    )
  return (
    <ul className="divide-y divide-border">
      {rules.map((rule) => (
        <li
          key={rule.id}
          className="flex flex-wrap items-start justify-between gap-3 py-4"
        >
          <div className="min-w-0 flex-1 space-y-1 text-sm">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">
                {rule.group_id === 0
                  ? t('budgetAllGroups')
                  : rule.group_id === 1
                    ? t('defaultGroup')
                    : rule.group_name}
              </span>
              <span className="break-all">
                {rule.model || t('budgetAllModels')}
              </span>
              <span className="text-xs text-muted-foreground">
                {t(rule.period === 'day' ? 'budgetDaily' : 'budgetMonthly')}
              </span>
              {(!rule.enabled ||
                rule.pending > 0 ||
                rule.used >= rule.limit) && (
                <Status
                  kind={
                    !rule.enabled
                      ? 'neutral'
                      : rule.pending > 0
                        ? 'warning'
                        : 'error'
                  }
                >
                  {t(
                    !rule.enabled
                      ? 'budgetDisabled'
                      : rule.pending > 0
                        ? 'budgetPending'
                        : 'budgetExhausted',
                  )}
                </Status>
              )}
            </div>
            <p className="tabular-nums">
              {t('budgetUsed', {
                used: number.format(rule.used / 1_000_000),
                limit: number.format(rule.limit / 1_000_000),
              })}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('budgetResets', { time: date.format(rule.reset_at * 1000) })}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('budgetTracking', {
                time: date.format(rule.created_at * 1000),
              })}
            </p>
            {rule.pending > 0 && (
              <p className="text-xs text-warning">
                {t('budgetPendingCount', { count: rule.pending })}
              </p>
            )}
          </div>
          {onEdit && (
            <Button variant="outline" size="sm" onClick={() => onEdit(rule)}>
              {t('budgetEdit')}
            </Button>
          )}
        </li>
      ))}
    </ul>
  )
}
