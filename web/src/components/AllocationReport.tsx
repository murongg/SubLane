import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  allocationOptions,
  refreshAllocation,
  reserveAllocation,
  allocationErrorKey,
  modeLabels,
} from '@/lib/allocations'
import { Button } from './ui/Button'
import { AllocationBalances } from './AllocationBalances'
import { AllocationSettlement } from './AllocationSettlement'
export function AllocationReport({ id }: { id: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(allocationOptions(id))
  const reload = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['allocation', id] }),
      client.invalidateQueries({ queryKey: ['own-allocations'] }),
    ])
  }
  const refresh = useMutation({
    mutationFn: () => refreshAllocation(id),
    onSuccess: reload,
  })
  const reserve = useMutation({
    mutationFn: (points: number) => reserveAllocation(id, points),
    onSuccess: reload,
  })
  if (query.isPending) return <p role="status">{t('allocationLoading')}</p>
  if (query.isError)
    return (
      <div role="alert">
        <p>{t('allocationFailed')}</p>
        <Button onClick={() => query.refetch()}>{t('reconnect')}</Button>
      </div>
    )
  const d = query.data
  return (
    <section className="max-w-4xl space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="break-words text-lg font-semibold">{d.name}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {d.team_name} · {d.group_name} · {t(modeLabels[d.config.mode])}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={query.isFetching || refresh.isPending}
          onClick={() =>
            d.config.mode === 'ratio' ? refresh.mutate() : query.refetch()
          }
        >
          {t(d.config.mode === 'ratio' ? 'allocationSync' : 'refresh')}
        </Button>
      </div>
      {refresh.isError && (
        <p role="alert" className="text-sm text-error">
          {t(allocationErrorKey(refresh.error))}
        </p>
      )}
      <AllocationBalances detail={d} />
      {d.unassigned > 0 && (
        <div className="space-y-3 border-t border-border pt-4">
          <p className="text-sm leading-6">
            {t('allocationUnassignedHint', { points: d.unassigned / 100 })}
          </p>
          <Button
            variant="outline"
            disabled={reserve.isPending}
            onClick={() => reserve.mutate(d.unassigned)}
          >
            {t('allocationReserve')}
          </Button>
          {reserve.isError && (
            <p role="alert" className="text-sm text-error">
              {t(allocationErrorKey(reserve.error))}
            </p>
          )}
        </div>
      )}
      {d.pending.length > 0 && (
        <section className="space-y-4 border-t border-border pt-5">
          <h3 className="font-medium">{t('allocationPending')}</h3>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('allocationSettlementHint')}
          </p>
          {d.pending.map((p) => (
            <AllocationSettlement
              key={p.request_id}
              id={id}
              entry={p}
              onSaved={reload}
            />
          ))}
        </section>
      )}
    </section>
  )
}
