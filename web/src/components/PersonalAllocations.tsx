import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ownAllocationOptions, modeLabels } from '@/lib/allocations'
import { Button } from './ui/Button'
import { AllocationBalances } from './AllocationBalances'
export function PersonalAllocations({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(ownAllocationOptions(client, userID))
  return (
    <section
      aria-labelledby="personal-allocations-title"
      className="space-y-4 rounded-xl border border-border bg-card p-5"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="personal-allocations-title" className="font-medium">
          {t('allocationMine')}
        </h2>
        <Button
          size="sm"
          variant="outline"
          disabled={query.isFetching}
          onClick={() => query.refetch()}
          aria-label={t('allocationRefreshMine')}
        >
          {t('refresh')}
        </Button>
      </div>
      {query.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('allocationLoading')}
        </p>
      ) : query.isError ? (
        <p role="alert" className="text-sm text-error">
          {t('allocationFailed')}
        </p>
      ) : query.data.schemes.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('allocationMineEmpty')}
        </p>
      ) : (
        <div className="divide-y divide-border">
          {query.data.schemes.map((d) => (
            <section key={d.id} className="space-y-3 py-4">
              <h3 className="break-words font-medium">{d.name}</h3>
              <p className="text-sm text-muted-foreground">
                {d.team_name} · {d.group_name} · {t(modeLabels[d.config.mode])}
              </p>
              <AllocationBalances detail={d} />
            </section>
          ))}
        </div>
      )}
    </section>
  )
}
