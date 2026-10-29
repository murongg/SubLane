import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { CalendarClock, ChevronDown, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { usageOptions } from '@/lib/usage'
import { runtimeOptions } from '@/lib/runtime'
import {
  resetOptions,
  resetCardsOptions,
  saveResetSettings,
  consumeResetCredit,
  resetErrorKey,
  resetReasons,
  type ResetConfig,
  type ResetState,
} from '@/lib/resets'
import { ResetCards } from './ResetCards'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

export function AccountResets({
  id,
  name,
  enabled,
}: {
  id: string
  name: string
  enabled: boolean
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(resetOptions(id))
  const confirmedAt = query.data?.history.find(
    (run) => run.outcome === 'success',
  )?.started_at
  useEffect(() => {
    if (!confirmedAt) return
    // Background redemptions must refresh an already-open usage card too.
    refreshAccountUsage(client, id).catch(() =>
      console.error('Could not refresh account usage after reset'),
    )
  }, [client, id, confirmedAt])
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: query.data?.time_zone ?? 'UTC',
  })
  return (
    <div className="space-y-1">
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogTrigger asChild>
          <Button variant="ghost" size="sm" className="-ml-2">
            <CalendarClock aria-hidden="true" />
            {t('resetSettings')}
          </Button>
        </DialogTrigger>
        <DialogContent
          className="max-h-[85svh] overflow-y-auto"
          onOpenAutoFocus={(event) => {
            event.preventDefault()
            heading.current?.focus()
          }}
          onInteractOutside={(event) => {
            if (busy) event.preventDefault()
          }}
          onEscapeKeyDown={(event) => {
            if (busy) event.preventDefault()
          }}
          showCloseButton={!busy}
        >
          <DialogHeader>
            <DialogTitle ref={heading} tabIndex={-1}>
              {t('resetSettings')}
            </DialogTitle>
            <DialogDescription>{name}</DialogDescription>
          </DialogHeader>
          {query.isPending ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('loading')}
            </p>
          ) : query.isError && !query.data ? (
            <div className="space-y-3">
              <p role="alert" className="text-sm text-error">
                {t('resetLoadFailed')}
              </p>
              <Button variant="outline" onClick={() => query.refetch()}>
                {t('resetRetry')}
              </Button>
            </div>
          ) : query.data ? (
            <>
              {query.isError && (
                <p role="alert" className="text-sm text-warning">
                  {t('resetLoadFailed')}
                </p>
              )}
              <ResetCards
                id={id}
                enabled={enabled}
                timeZone={query.data.time_zone}
                pending={query.data.pending}
              />
              <ResetForm
                id={id}
                enabled={enabled}
                state={query.data}
                setBusy={setBusy}
              />
            </>
          ) : null}
        </DialogContent>
      </Dialog>
      {query.data?.pending ? (
        <p className="text-xs text-warning">{t('resetResultPending')}</p>
      ) : query.data?.next_run_at ? (
        <p className="text-xs text-muted-foreground">
          {t('resetNext', {
            date: dates.format(query.data.next_run_at * 1000),
          })}
        </p>
      ) : null}
      {query.data?.config.exhausted && (
        <p className="text-xs text-muted-foreground">{t('resetAutoSummary')}</p>
      )}
    </div>
  )
}

function refreshAccountUsage(client: QueryClient, id: string) {
  return Promise.all([
    client.invalidateQueries({ queryKey: usageOptions(id).queryKey }),
    client.invalidateQueries({ queryKey: runtimeOptions.queryKey }),
    client.invalidateQueries({ queryKey: resetCardsOptions(id).queryKey }),
  ])
}

function Choice({
  label,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (value: string) => void
  disabled: boolean
}) {
  const id = useId()
  return (
    <div className="space-y-2">
      <span id={id} className="text-sm font-medium">
        {label}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            className="w-full justify-between"
            aria-labelledby={id}
            disabled={disabled}
          >
            {options.find((option) => option.value === value)?.label}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuRadioGroup value={value} onValueChange={onChange}>
            {options.map((option) => (
              <DropdownMenuRadioItem key={option.value} value={option.value}>
                {option.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function ResetForm({
  id,
  enabled,
  state,
  setBusy,
}: {
  id: string
  enabled: boolean
  state: ResetState
  setBusy: (busy: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const prefix = useId()
  const [config, setConfig] = useState<ResetConfig>(state.config)
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState(false)
  const change = (patch: Partial<ResetConfig>) => {
    setSaved(false)
    setConfig((current) => ({ ...current, ...patch }))
  }
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: state.time_zone,
  })
  const save = useMutation({
    mutationFn: () => saveResetSettings(id, config),
    onMutate: () => setBusy(true),
    onSuccess: (data) => {
      client.setQueryData(resetOptions(id).queryKey, data)
      setConfig(data.config)
      setSaved(true)
    },
    onSettled: () => setBusy(false),
  })
  const redemption = useMutation({
    mutationFn: (operation: 'consume' | 'retry') =>
      consumeResetCredit(id, operation),
    onMutate: () => setBusy(true),
    onSuccess: async (data) => {
      client.setQueryData(resetOptions(id).queryKey, data)
      await refreshAccountUsage(client, id)
    },
    onSettled: () => setBusy(false),
  })
  const busy = save.isPending || redemption.isPending
  const dirty = JSON.stringify(config) !== JSON.stringify(state.config)
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (busy) return
    const valid =
      Number.isInteger(config.daily_limit) &&
      config.daily_limit >= 1 &&
      config.daily_limit <= 100 &&
      (!config.scheduled ||
        (/^\d{2}:\d{2}$/.test(config.time) &&
          (config.frequency !== 'once' || config.date !== '')))
    setInvalid(!valid)
    if (valid) save.mutate()
  }
  const frequencies = [
    { value: 'once', label: t('resetOnce') },
    { value: 'daily', label: t('resetDaily') },
    { value: 'weekly', label: t('resetWeekly') },
  ]
  const weekdays = Array.from({ length: 7 }, (_, index) => ({
    value: String(index),
    label: new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
      weekday: 'long',
      timeZone: 'UTC',
    }).format(new Date(Date.UTC(2026, 0, 4 + index))),
  }))
  return (
    <form className="space-y-5" onSubmit={submit} noValidate>
      <p className="text-sm leading-6 text-muted-foreground">
        {t('resetDescription')}
      </p>
      <fieldset disabled={busy} className="space-y-4 border-0 p-0">
        <label className="flex min-h-10 items-center gap-3 text-sm font-medium">
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={config.scheduled}
            onChange={(event) => change({ scheduled: event.target.checked })}
          />
          {t('resetScheduled')}
        </label>
        {config.scheduled && (
          <div className="grid gap-4 pl-7 sm:grid-cols-2">
            <Choice
              label={t('resetFrequency')}
              value={config.frequency}
              options={frequencies}
              onChange={(value) =>
                change({ frequency: value as ResetConfig['frequency'] })
              }
              disabled={busy}
            />
            {config.frequency === 'weekly' && (
              <Choice
                label={t('resetWeekday')}
                value={String(config.weekday)}
                options={weekdays}
                onChange={(value) => change({ weekday: Number(value) })}
                disabled={busy}
              />
            )}
            {config.frequency === 'once' && (
              <div className="space-y-2">
                <label
                  htmlFor={`${prefix}-date`}
                  className="text-sm font-medium"
                >
                  {t('resetDate')}
                </label>
                <Input
                  id={`${prefix}-date`}
                  type="date"
                  value={config.date}
                  onChange={(event) => change({ date: event.target.value })}
                />
              </div>
            )}
            <div className="space-y-2">
              <label htmlFor={`${prefix}-time`} className="text-sm font-medium">
                {t('resetTime')}
              </label>
              <Input
                id={`${prefix}-time`}
                type="time"
                value={config.time}
                onChange={(event) => change({ time: event.target.value })}
              />
            </div>
            <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">
              {t('resetZone', { zone: state.time_zone })} {t('resetMissedHint')}
            </p>
          </div>
        )}
        <div className="space-y-1">
          <label className="flex min-h-10 items-center gap-3 text-sm font-medium">
            <input
              type="checkbox"
              className="size-4 accent-primary"
              checked={config.exhausted}
              onChange={(event) => change({ exhausted: event.target.checked })}
            />
            {t('resetExhausted')}
          </label>
          <p className="pl-7 text-xs leading-5 text-muted-foreground">
            {t('resetExhaustedHint')}
          </p>
        </div>
        <div className="space-y-2 border-t border-border pt-4">
          <label htmlFor={`${prefix}-limit`} className="text-sm font-medium">
            {t('resetDailyLimit')}
          </label>
          <Input
            id={`${prefix}-limit`}
            type="number"
            min={1}
            max={100}
            step={1}
            value={Number.isNaN(config.daily_limit) ? '' : config.daily_limit}
            onChange={(event) =>
              change({
                daily_limit:
                  event.target.value === '' ? NaN : Number(event.target.value),
              })
            }
            aria-invalid={invalid}
            aria-describedby={`${prefix}-limit-hint`}
          />
          <p
            id={`${prefix}-limit-hint`}
            className="text-xs leading-5 text-muted-foreground"
          >
            {t('resetLimitHint', {
              used: state.used_today,
              limit: state.config.daily_limit,
            })}
          </p>
        </div>
      </fieldset>
      {state.next_run_at > 0 && (
        <p className="text-sm text-muted-foreground">
          {t('resetNext', { date: dates.format(state.next_run_at * 1000) })}
        </p>
      )}
      {(invalid || save.isError || redemption.isError) && (
        <p role="alert" className="text-sm text-error">
          {t(
            invalid
              ? 'resetInvalid'
              : resetErrorKey(save.error ?? redemption.error),
          )}
        </p>
      )}
      {saved && (
        <p role="status" className="text-sm text-success">
          {t('resetSaved')}
        </p>
      )}
      <DialogFooter>
        <Button type="submit" disabled={busy}>
          {save.isPending && (
            <LoaderCircle
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          )}
          {t('resetSave')}
        </Button>
      </DialogFooter>
      <section
        className="space-y-3 border-t border-border pt-4"
        aria-label={t('resetActions')}
      >
        {state.pending ? (
          <p className="text-sm leading-6 text-warning">
            {t('resetPendingHint')}
          </p>
        ) : (
          <p className="text-xs leading-5 text-muted-foreground">
            {t('resetManualHint')}
          </p>
        )}
        {dirty && (
          <p className="text-xs text-muted-foreground">{t('resetSaveFirst')}</p>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={
            busy ||
            !enabled ||
            dirty ||
            (!state.pending && state.used_today >= state.config.daily_limit)
          }
          onClick={() => redemption.mutate(state.pending ? 'retry' : 'consume')}
        >
          {redemption.isPending && (
            <LoaderCircle
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          )}
          {t(state.pending ? 'resetCheckPending' : 'resetUseNow')}
        </Button>
      </section>
      <section
        className="space-y-3 border-t border-border pt-4"
        aria-labelledby={`${prefix}-history`}
      >
        <h3 id={`${prefix}-history`} className="text-sm font-medium">
          {t('resetHistory')}
        </h3>
        {state.history.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('resetHistoryEmpty')}
          </p>
        ) : (
          <ol className="space-y-3">
            {state.history.map((run, index) => (
              <li
                key={`${run.started_at}-${index}`}
                className="space-y-1 text-sm"
              >
                <p
                  className={
                    run.outcome === 'pending'
                      ? 'text-warning'
                      : run.outcome === 'failed'
                        ? 'text-error'
                        : ''
                  }
                >
                  {t(resetReasons[run.reason] ?? 'resetFailed')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t(
                    run.trigger === 'scheduled'
                      ? 'resetTriggerScheduled'
                      : run.trigger === 'exhausted'
                        ? 'resetTriggerExhausted'
                        : 'resetTriggerManual',
                  )}{' '}
                  · {dates.format(run.started_at * 1000)}
                </p>
              </li>
            ))}
          </ol>
        )}
      </section>
    </form>
  )
}
