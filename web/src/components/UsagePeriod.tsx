import { useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { DateRange, StatisticsPeriod } from '@/lib/statistics'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import { Input } from './ui/Input'

const periods = [1, 7, 30, 90]

export function UsagePeriod({
  value,
  onChange,
  timeZone,
  custom,
}: {
  value: StatisticsPeriod
  onChange: (period: StatisticsPeriod) => void
  timeZone: string
  custom: boolean
}) {
  const { t, i18n } = useTranslation()
  const trigger = useRef<HTMLButtonElement>(null)
  const [draft, setDraft] = useState<DateRange | null>(null)
  const preset = typeof value === 'number'
  const periodLabel = (days: number) =>
    days === 1 ? t('usageToday') : t('usageLastDays', { days })
  const format = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeZone: 'UTC',
  })
  const label = preset
    ? periodLabel(value)
    : format.formatRange(new Date(value.startDate), new Date(value.endDate))

  // These UTC Date objects represent civil dates for the inputs, independent of DST.
  const today = new Intl.DateTimeFormat('sv-SE', { timeZone }).format(
    new Date(),
  )
  const oldest = new Date(today)
  oldest.setUTCDate(oldest.getUTCDate() - 89)
  const earliest = oldest.toISOString().slice(0, 10)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={trigger}
            variant="outline"
            aria-label={t('usagePeriod')}
            aria-description={label}
          >
            {label}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          onCloseAutoFocus={(event) => {
            if (draft) event.preventDefault()
          }}
        >
          <DropdownMenuRadioGroup
            value={preset ? String(value) : ''}
            onValueChange={(days) => onChange(Number(days))}
          >
            {periods.map((days) => (
              <DropdownMenuRadioItem key={days} value={String(days)}>
                {periodLabel(days)}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          {custom && (
            <DropdownMenuItem
              onSelect={() =>
                setDraft(preset ? { startDate: today, endDate: today } : value)
              }
            >
              {t('usageCustomDates')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
      >
        <DialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault()
            trigger.current?.focus()
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('usageCustomDates')}</DialogTitle>
            <DialogDescription>
              {t('usageDateZone', { zone: timeZone })}
            </DialogDescription>
          </DialogHeader>
          {draft && (
            <form
              className="space-y-4"
              onSubmit={(event) => {
                event.preventDefault()
                onChange(draft)
                setDraft(null)
              }}
            >
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="space-y-1.5 text-sm font-medium">
                  <span>{t('usageStartDate')}</span>
                  <Input
                    type="date"
                    required
                    min={earliest}
                    max={today}
                    value={draft.startDate}
                    onChange={(event) =>
                      setDraft({ ...draft, startDate: event.target.value })
                    }
                  />
                </label>
                <label className="space-y-1.5 text-sm font-medium">
                  <span>{t('usageEndDate')}</span>
                  <Input
                    type="date"
                    required
                    min={draft.startDate || earliest}
                    max={today}
                    value={draft.endDate}
                    onChange={(event) =>
                      setDraft({ ...draft, endDate: event.target.value })
                    }
                  />
                </label>
              </div>
              <DialogFooter>
                <DialogClose asChild>
                  <Button type="button" variant="outline">
                    {t('cancel')}
                  </Button>
                </DialogClose>
                <Button type="submit">{t('requestApplyFilters')}</Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
