import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  type AllocationPending,
  settleAllocation,
  allocationValue,
  parseAllocationValue,
  allocationErrorKey,
} from '@/lib/allocations'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
export function AllocationSettlement({
  id,
  entry,
  onSaved,
}: {
  id: number
  entry: AllocationPending
  onSaved: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [invalid, setInvalid] = useState(false)
  const [open, setOpen] = useState(false)
  const mutation = useMutation({
    mutationFn: (input: Parameters<typeof settleAllocation>[1]) =>
      settleAllocation(id, input),
    onSuccess: onSaved,
  })
  return (
    <div className="space-y-3 border-b border-border pb-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="break-all text-sm font-medium">{entry.request_id}</p>
          <p className="text-xs text-muted-foreground">
            {entry.model} ·{' '}
            {t(
              entry.state === 'observed'
                ? 'allocationAwaitingSync'
                : 'allocationPending',
            )}
          </p>
        </div>
        {!open && (
          <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
            {t('allocationCorrect')}
          </Button>
        )}
      </div>
      {open && (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (mutation.isPending) return
            const form = new FormData(e.currentTarget)
            const points: Record<number, number> = {}
            let input = entry.input,
              output = entry.output,
              cached = entry.cached
            let bad = false
            if (entry.mode === 'ratio') {
              for (const d of entry.debits) {
                const v = d.reconciled
                  ? d.points
                  : parseAllocationValue(
                      String(form.get(`window-${d.window_id}`) ?? ''),
                      'ratio',
                    )
                if (v === null || v > 10000) bad = true
                else points[d.window_id] = v
              }
            } else {
              input =
                parseAllocationValue(
                  String(form.get('input') ?? ''),
                  'tokens',
                ) ?? -1
              output =
                parseAllocationValue(
                  String(form.get('output') ?? ''),
                  'tokens',
                ) ?? -1
              cached =
                parseAllocationValue(
                  String(form.get('cached') ?? ''),
                  'tokens',
                ) ?? -1
              bad =
                input < entry.input ||
                output < entry.output ||
                cached < 0 ||
                cached > input
            }
            setInvalid(bad)
            if (!bad)
              mutation.mutate({
                request_id: entry.request_id,
                input,
                output,
                cached,
                points,
              })
          }}
        >
          <fieldset
            disabled={mutation.isPending}
            className="grid gap-3 sm:grid-cols-3"
          >
            {entry.mode === 'ratio'
              ? entry.debits.map((d) => (
                  <div key={d.window_id} className="space-y-2">
                    <label
                      htmlFor={`${entry.request_id}-${d.window_id}`}
                      className="text-sm"
                    >
                      {t(
                        d.kind === 'primary'
                          ? 'allocationPrimary'
                          : 'allocationSecondary',
                      )}{' '}
                      · {t('allocationPoints')}
                    </label>
                    <Input
                      id={`${entry.request_id}-${d.window_id}`}
                      name={`window-${d.window_id}`}
                      inputMode="decimal"
                      disabled={d.reconciled}
                      defaultValue={
                        d.reconciled ? allocationValue(d.points, 'ratio') : ''
                      }
                    />
                  </div>
                ))
              : (['input', 'output', 'cached'] as const).map((key) => (
                  <div key={key} className="space-y-2">
                    <label
                      htmlFor={`${entry.request_id}-${key}`}
                      className="text-sm"
                    >
                      {t(
                        key === 'input'
                          ? 'allocationInputM'
                          : key === 'output'
                            ? 'allocationOutputM'
                            : 'allocationCachedM',
                      )}
                    </label>
                    <Input
                      id={`${entry.request_id}-${key}`}
                      name={key}
                      inputMode="decimal"
                      defaultValue={allocationValue(entry[key], 'tokens')}
                    />
                  </div>
                ))}
          </fieldset>
          {(invalid || mutation.isError) && (
            <p role="alert" className="text-sm text-error">
              {t(
                invalid
                  ? 'allocationInvalid'
                  : allocationErrorKey(mutation.error!),
              )}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => setOpen(false)}
            >
              {t('cancel')}
            </Button>
            <Button disabled={mutation.isPending}>
              {t('allocationSettle')}
            </Button>
          </div>
        </form>
      )}
    </div>
  )
}
