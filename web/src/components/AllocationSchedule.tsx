import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  allocationErrorKey,
  changeSchedule,
  modeLabels,
  previewScheme,
  type Scheme,
} from '@/lib/allocations'
import { ApiError } from '@/lib/request'
import { formatInstanceDate, useTimeZone } from '@/lib/timezone'
import { Button } from './ui/Button'

type Prepared = { source: Scheme; action: 'cancel' | 'reschedule'; at?: number }

export function AllocationSchedule({
  scheme,
  onChanged,
}: {
  scheme: Scheme
  onChanged: () => Promise<unknown>
}) {
  const { t, i18n } = useTranslation()
  const zone = useTimeZone()
  // Polling may replace props; confirmation must keep the reviewed source versions.
  const [prepared, setPrepared] = useState<Prepared | null>(null)
  const prepare = useMutation({
    mutationFn: async () => {
      const source = scheme
      const preview = await previewScheme(source.id, 'reschedule')
      if (
        preview.write_conditions.expected_current_revision_id !==
          source.write_conditions?.expected_current_revision_id ||
        preview.write_conditions.expected_next_revision_id !==
          source.write_conditions?.expected_next_revision_id
      ) {
        throw new ApiError('allocation_version_conflict', 409)
      }
      return {
        source,
        action: 'reschedule' as const,
        at: preview.proposed_effective_at,
      }
    },
    onSuccess: setPrepared,
  })
  const mutation = useMutation({
    mutationFn: (value: Prepared) =>
      changeSchedule(value.source, value.action, value.at),
    onSuccess: async () => {
      setPrepared(null)
      await onChanged()
    },
  })
  if (!scheme.next || !scheme.write_conditions) return null
  const busy = prepare.isPending || mutation.isPending
  const error = mutation.error ?? prepare.error
  const source = prepared?.source ?? scheme
  const date = (at: number) =>
    formatInstanceDate(at * 1000, i18n.resolvedLanguage ?? 'en', zone, {
      dateStyle: 'medium',
      timeStyle: 'short',
    })
  return (
    <div className="space-y-3">
      {!prepared ? (
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => {
              mutation.reset()
              prepare.mutate()
            }}
          >
            {t('allocationReschedule')}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={
              busy ||
              source.write_conditions?.expected_current_revision_id === null
            }
            onClick={() => {
              mutation.reset()
              prepare.reset()
              setPrepared({ source: scheme, action: 'cancel' })
            }}
          >
            {t('allocationCancelSchedule')}
          </Button>
        </div>
      ) : (
        <div className="space-y-3 border-t border-border pt-3">
          <p className="text-sm text-muted-foreground">
            {t('allocationScheduled', {
              mode: t(modeLabels[source.next!.config.mode]),
              date: date(source.next!.effective_at),
            })}
          </p>
          {prepared.at !== undefined && (
            <p role="status" className="text-sm">
              {t('allocationEditEffectivePreview', { date: date(prepared.at) })}
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              disabled={busy}
              onClick={() => mutation.mutate(prepared)}
            >
              {t(
                prepared.action === 'reschedule'
                  ? 'allocationConfirmStart'
                  : 'allocationCancelSchedule',
              )}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setPrepared(null)
                mutation.reset()
                prepare.reset()
              }}
            >
              {t('cancel')}
            </Button>
          </div>
        </div>
      )}
      {source.write_conditions?.expected_current_revision_id === null && (
        <p className="text-xs leading-5 text-muted-foreground">
          {t('allocationInitialScheduleRequired')}
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-error">
          {t(allocationErrorKey(error))}
        </p>
      )}
    </div>
  )
}
