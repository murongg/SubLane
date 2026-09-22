import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ownBudgetOptions } from '@/lib/budgets'
import { Button } from './ui/Button'
import { BudgetList } from './BudgetList'

export function PersonalBudgets({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(ownBudgetOptions(client, userID))
  return (
    <section
      className="rounded-xl border border-border bg-card px-5 py-4"
      aria-labelledby="personal-budgets-title"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="personal-budgets-title" className="font-medium">
          {t('tokenBudgets')}
        </h2>
        <Button
          aria-label={t('budgetRefresh')}
          size="sm"
          variant="outline"
          disabled={query.isFetching}
          onClick={() => query.refetch()}
        >
          {t('refresh')}
        </Button>
      </div>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t('budgetsPersonalHint')}
      </p>
      {query.isPending ? (
        <p role="status" className="py-4 text-sm text-muted-foreground">
          {t('budgetsLoading')}
        </p>
      ) : query.isError ? (
        <p role="alert" className="py-4 text-sm text-error">
          {t('budgetsLoadFailed')}
        </p>
      ) : (
        <BudgetList rules={query.data.rules.filter((rule) => rule.enabled)} />
      )}
    </section>
  )
}
