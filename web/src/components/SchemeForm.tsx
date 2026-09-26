import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { catalogOptions } from '@/lib/catalog'
import { useTimeZone } from '@/lib/timezone'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'
import {
  type Scheme,
  type SchemeInput,
  type AllocationMode,
  modeLabels,
  parseAllocationValue,
  allocationValue,
  lookupModelPrice,
  lookupModelPrices,
} from '@/lib/allocations'
import { Button } from './ui/Button'
import { Input } from './ui/Input'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

function SchemeChoice({
  id,
  label,
  value,
  options,
  disabled,
  placeholder,
  onChange,
}: {
  id: string
  label: string
  value: string
  options: { value: string; label: string }[]
  disabled: boolean
  placeholder?: string
  onChange: (value: string) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          className="w-full min-w-0 justify-between gap-3"
          aria-label={label}
          disabled={disabled}
        >
          <span className="truncate">
            {options.find((option) => option.value === value)?.label ??
              placeholder}
          </span>
          <ChevronDown className="size-4 shrink-0" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-h-72 w-[var(--radix-dropdown-menu-trigger-width)] overflow-y-auto"
      >
        <DropdownMenuRadioGroup value={value} onValueChange={onChange}>
          {options.map((option) => (
            <DropdownMenuRadioItem key={option.value} value={option.value}>
              {option.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
function ModelChoice({
  id,
  groupID,
  value,
  excluded,
  pending,
  onChange,
}: {
  id: string
  groupID: number
  value: string
  excluded: string[]
  pending: boolean
  onChange: (model: string) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(catalogOptions(client, { kind: 'group', id: groupID }))
  // Keep a saved model visible even if it is absent from the current pool catalog.
  const models = [
    ...new Set([...(value ? [value] : []), ...(query.data?.models ?? [])]),
  ].filter((model) => model === value || !excluded.includes(model))
  return (
    <div className="min-w-0 flex-1 space-y-2">
      <SchemeChoice
        id={id}
        label={t('allocationModelID')}
        value={value}
        placeholder={t(
          query.isPending ? 'catalogLoading' : 'allocationChooseModel',
        )}
        options={models.map((model) => ({ value: model, label: model }))}
        disabled={pending || models.length === 0}
        onChange={onChange}
      />
      {(query.isError || query.data?.refresh_failed) && (
        <p role="status" className="text-xs text-warning">
          {t('catalogLoadFailed')}
        </p>
      )}
      {query.data?.partial && (
        <p role="status" className="text-xs text-muted-foreground">
          {t('catalogPartial')}
        </p>
      )}
      {query.data && !query.data.known && (
        <p role="status" className="text-xs text-muted-foreground">
          {t('catalogUnknownHint')}
        </p>
      )}
      {query.data?.known && query.data.models.length === 0 && (
        <p role="status" className="text-xs text-muted-foreground">
          {t('catalogEmptyGroup')}
        </p>
      )}
      {(query.isError ||
        query.data?.refresh_failed ||
        query.data?.models.length === 0) && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={pending || query.isFetching}
          onClick={() => query.refetch()}
        >
          {t('catalogReload')}
        </Button>
      )}
    </div>
  )
}
type RateDraft = {
  model: string
  input: string
  cached: string
  output: string
  priceState?: 'loading' | 'missing' | 'failed'
}
function AddAllModels({
  groupID,
  rates,
  pending,
  onAdd,
}: {
  groupID: number
  rates: RateDraft[]
  pending: boolean
  onAdd: (models: string[]) => Promise<void>
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(catalogOptions(client, { kind: 'group', id: groupID }))
  const selected = new Set(rates.map((rate) => rate.model))
  const missing = [...new Set(query.data?.models ?? [])].filter(
    (model) => !selected.has(model),
  )
  const overLimit = missing.length > 128 - rates.length
  const unavailable =
    query.isPending ||
    query.isError ||
    query.isFetching ||
    !query.data?.known ||
    !query.data.usable ||
    query.data.partial ||
    query.data.stale ||
    query.data.refreshing ||
    query.data.refresh_failed
  const catalogStatus =
    query.isError || query.data?.refresh_failed
      ? 'catalogLoadFailed'
      : query.data?.partial
        ? 'catalogPartial'
        : query.data && !query.data.known
          ? 'catalogUnknownHint'
          : query.data?.known && query.data.models.length === 0
            ? 'catalogEmptyGroup'
            : query.data?.stale
              ? 'catalogStale'
              : query.data?.refreshing
                ? 'catalogRefreshing'
                : null
  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        disabled={pending || unavailable || overLimit || missing.length === 0}
        onClick={() => onAdd(missing)}
      >
        {t('allocationAddAllRates', { count: missing.length })}
      </Button>
      {overLimit && (
        <p role="status" className="text-xs text-muted-foreground">
          {t('allocationRateLimit')}
        </p>
      )}
      {rates.length === 0 && catalogStatus && (
        <p
          role="status"
          className={
            catalogStatus === 'catalogLoadFailed'
              ? 'text-xs text-warning'
              : 'text-xs text-muted-foreground'
          }
        >
          {t(catalogStatus)}
        </p>
      )}
      {(query.isError ||
        query.data?.refresh_failed ||
        query.data?.models.length === 0) &&
        rates.length === 0 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={pending || query.isFetching}
            onClick={() => query.refetch()}
          >
            {t('catalogReload')}
          </Button>
        )}
    </div>
  )
}
export function SchemeForm({
  groups,
  groupID,
  onGroupChange,
  members,
  scheme,
  onSubmit,
  onCancel,
  pending,
}: {
  groups: { id: number; name: string; enabled: boolean }[]
  groupID: number
  onGroupChange: (id: number) => void
  members: { id: number; username: string }[]
  scheme?: Scheme
  onSubmit: (input: SchemeInput) => void
  onCancel: () => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const timeZone = useTimeZone()
  const initial = scheme?.next?.config ?? scheme?.config
  const [name, setName] = useState(scheme?.name ?? '')
  const [mode, setMode] = useState<AllocationMode>(initial?.mode ?? 'ratio')
  const [ratioUnit, setRatioUnit] = useState<'amount' | 'tokens'>(
    initial?.ratio_unit ?? 'tokens',
  )
  const [period, setPeriod] = useState(
    initial?.period === 'day' ? 'day' : 'month',
  )
  const [resetTime, setResetTime] = useState(initial?.reset_time ?? '00:00')
  const [resetDay, setResetDay] = useState(String(initial?.reset_day ?? 1))
  const [enabled, setEnabled] = useState(scheme?.enabled ?? true)
  const [ratioTotal, setRatioTotal] = useState(
    initial?.total
      ? allocationValue(initial.total, initial.ratio_unit ?? 'tokens')
      : '',
  )
  const [startNext, setStartNext] = useState(false)
  const [values, setValues] = useState<Record<number, string>>(
    Object.fromEntries(
      initial?.members.map((m) => [
        m.user_id,
        allocationValue(m.limit, initial.mode),
      ]) ?? [],
    ),
  )
  const [rates, setRates] = useState<RateDraft[]>(
    initial?.rates.map((r) => ({
      model: r.model,
      input: allocationValue(r.input, 'amount'),
      cached: allocationValue(r.cached, 'amount'),
      output: allocationValue(r.output, 'amount'),
    })) ?? [],
  )
  const [advancedRates, setAdvancedRates] = useState(
    mode !== 'ratio' || (!!scheme && (initial?.rates.length ?? 0) > 0),
  )
  const [invalid, setInvalid] = useState(false)
  const shareMode = mode === 'ratio'
  const tokenBased = mode === 'tokens' || (shareMode && ratioUnit === 'tokens')
  const total = members.reduce(
    (sum, m) => sum + (parseAllocationValue(values[m.id] ?? '', mode) ?? 0),
    0,
  )
  const changeMode = (next: AllocationMode) => {
    if (next !== mode) {
      setMode(next)
      setAdvancedRates(next !== 'ratio')
      setValues({})
      setInvalid(false)
    }
  }
  const updateRate = (i: number, field: keyof RateDraft, value: string) =>
    setRates((all) =>
      all.map((r, n) => (n === i ? { ...r, [field]: value } : r)),
    )
  const fillPrice = async (index: number, model: string) => {
    const draft: RateDraft = {
      model,
      input: '',
      cached: '',
      output: '',
      priceState: 'loading',
    }
    setRates((all) => all.map((rate, i) => (i === index ? draft : rate)))
    try {
      const price = await lookupModelPrice(model)
      // Match the exact selection, not its array index: rows can be removed or
      // another model selected before this request finishes.
      setRates((all) =>
        all.map((rate) =>
          rate === draft
            ? {
                model,
                input: price ? allocationValue(price.input, 'amount') : '',
                cached: price ? allocationValue(price.cached, 'amount') : '',
                output: price ? allocationValue(price.output, 'amount') : '',
                priceState: price ? undefined : 'missing',
              }
            : rate,
        ),
      )
    } catch {
      setRates((all) =>
        all.map((rate) =>
          rate === draft ? { ...rate, priceState: 'failed' } : rate,
        ),
      )
    }
  }
  const fillAllPrices = async (models: string[]) => {
    const drafts: RateDraft[] = models.map((model) => ({
      model,
      input: '',
      cached: '',
      output: '',
      priceState: 'loading',
    }))
    setRates((all) => [...all, ...drafts])
    try {
      const prices = await lookupModelPrices(models)
      setRates((all) =>
        all.map((rate) => {
          // Removed or reselected rows no longer match their original drafts.
          const index = drafts.indexOf(rate)
          if (index < 0) return rate
          const price = prices[models[index]]
          return {
            model: rate.model,
            input: price ? allocationValue(price.input, 'amount') : '',
            cached: price ? allocationValue(price.cached, 'amount') : '',
            output: price ? allocationValue(price.output, 'amount') : '',
            priceState: price ? undefined : 'missing',
          }
        }),
      )
    } catch {
      setRates((all) =>
        all.map((rate) =>
          drafts.includes(rate) ? { ...rate, priceState: 'failed' } : rate,
        ),
      )
    }
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (
      pending ||
      (!tokenBased && rates.some((rate) => rate.priceState === 'loading'))
    )
      return
    const shares = members
      .filter((m) => (values[m.id] ?? '').trim() !== '')
      .map((m) => ({
        user_id: m.id,
        limit: parseAllocationValue(values[m.id], mode) ?? -1,
      }))
    const parsedRates = tokenBased
      ? []
      : mode === 'ratio' && !advancedRates
        ? // Editing shares must preserve the revision's pricing snapshot even
          // while pricing controls are collapsed. New resources resolve prices automatically.
          initial?.mode === 'ratio' && initial.ratio_unit === 'amount'
          ? initial.rates
          : []
        : rates.map((r) => ({
            model: r.model.trim(),
            input: parseAllocationValue(r.input, 'amount') ?? -1,
            cached: parseAllocationValue(r.cached, 'amount') ?? -1,
            output: parseAllocationValue(r.output, 'amount') ?? -1,
          }))
    const totalBudget = shareMode
      ? (parseAllocationValue(ratioTotal, ratioUnit) ?? -1)
      : 0
    const bad =
      !name.trim() ||
      !groupID ||
      !/^(?:[01][0-9]|2[0-3]):[0-5][0-9]$/.test(resetTime) ||
      (period === 'month' &&
        (!/^\d{1,2}$/.test(resetDay) ||
          Number(resetDay) < 1 ||
          Number(resetDay) > 31)) ||
      shares.length === 0 ||
      shares.some((m) => m.limit <= 0) ||
      (shareMode && total > 10000) ||
      (shareMode &&
        (totalBudget <= 0 ||
          totalBudget > 1_000_000_000_000 ||
          shares.some(
            (m) => Math.floor((totalBudget * m.limit) / 10000) === 0,
          ))) ||
      ((mode === 'amount' ||
        (shareMode && ratioUnit === 'amount' && advancedRates)) &&
        (parsedRates.length === 0 ||
          parsedRates.some(
            (r) =>
              !r.model ||
              r.input <= 0 ||
              r.output <= 0 ||
              r.cached < 0 ||
              Math.max(r.input, r.output, r.cached) > 1_000_000_000,
          )))
    setInvalid(bad)
    if (bad) return
    onSubmit({
      name: name.trim(),
      group_id: groupID,
      enabled,
      start_next: startNext,
      config: {
        mode,
        period: period as 'day' | 'month',
        reset_time: resetTime,
        ...(period === 'month' ? { reset_day: Number(resetDay) } : {}),
        members: shares,
        rates: parsedRates,
        ...(shareMode ? { ratio_unit: ratioUnit, total: totalBudget } : {}),
      },
    })
  }
  return (
    <form onSubmit={submit} className="space-y-6" noValidate>
      <fieldset disabled={pending} className="space-y-6">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2 sm:col-span-2">
            <label htmlFor="scheme-name" className="text-sm font-medium">
              {t('allocationName')}
            </label>
            <Input
              id="scheme-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={64}
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="scheme-pool" className="text-sm font-medium">
              {t('keyGroup')}
            </label>
            <SchemeChoice
              id="scheme-pool"
              label={t('keyGroup')}
              value={String(groupID)}
              disabled={!!scheme || pending}
              options={groups.map((pool) => ({
                value: String(pool.id),
                label: pool.name,
              }))}
              onChange={(value) => {
                onGroupChange(Number(value))
                setValues({})
              }}
            />
          </div>
        </div>
        <p className="text-sm leading-6 text-muted-foreground">
          {t('allocationExclusiveHint')}
        </p>
        <fieldset className="space-y-3">
          <legend className="text-sm font-medium">{t('allocationMode')}</legend>
          <div className="flex flex-wrap gap-x-6 gap-y-3">
            {(['ratio', 'amount', 'tokens'] as const).map((value) => (
              <label key={value} className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="allocation-mode"
                  value={value}
                  checked={mode === value}
                  onChange={() => changeMode(value)}
                  className="size-4 accent-primary"
                />
                {t(modeLabels[value])}
              </label>
            ))}
          </div>
          <p className="text-sm leading-6 text-muted-foreground">
            {t(
              mode === 'ratio'
                ? 'allocationRatioHint'
                : mode === 'amount'
                  ? 'allocationAmountHint'
                  : 'allocationTokensHint',
            )}
          </p>
        </fieldset>
        {shareMode && (
          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">
              {t('allocationRatioUnit')}
            </legend>
            <div className="flex flex-wrap gap-x-6 gap-y-3">
              {(['tokens', 'amount'] as const).map((unit) => (
                <label key={unit} className="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="allocation-ratio-unit"
                    value={unit}
                    checked={ratioUnit === unit}
                    onChange={() => {
                      if (ratioUnit !== unit) {
                        setRatioUnit(unit)
                        setRatioTotal('')
                        setAdvancedRates(false)
                      }
                    }}
                    className="size-4 accent-primary"
                  />
                  {t(
                    unit === 'tokens'
                      ? 'allocationRatioTokens'
                      : 'allocationRatioAmount',
                  )}
                </label>
              ))}
            </div>
          </fieldset>
        )}
        <div className="grid max-w-xl gap-4 sm:grid-cols-2">
          {shareMode && (
            <div className="space-y-2">
              <label
                htmlFor="scheme-token-total"
                className="text-sm font-medium"
              >
                {t('allocationTotalBudget')}
              </label>
              <div className="flex items-center gap-2">
                <Input
                  id="scheme-token-total"
                  value={ratioTotal}
                  onChange={(e) => setRatioTotal(e.target.value)}
                  inputMode="decimal"
                  className="min-w-0 text-right tabular-nums"
                />
                <span className="text-sm text-muted-foreground">
                  {ratioUnit === 'amount' ? 'USD' : 'M'}
                </span>
              </div>
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="scheme-period" className="text-sm font-medium">
              {t('allocationPeriod')}
            </label>
            <SchemeChoice
              id="scheme-period"
              label={t('allocationPeriod')}
              value={period}
              disabled={pending}
              options={[
                { value: 'day', label: t('periodDaily', { zone: timeZone }) },
                {
                  value: 'month',
                  label: t('periodMonthly', { zone: timeZone }),
                },
              ]}
              onChange={setPeriod}
            />
          </div>
          {period === 'month' && (
            <div className="space-y-2">
              <label htmlFor="scheme-reset-day" className="text-sm font-medium">
                {t('allocationResetDay')}
              </label>
              <Input
                id="scheme-reset-day"
                type="number"
                min={1}
                max={31}
                step={1}
                value={resetDay}
                onChange={(e) => setResetDay(e.target.value)}
                inputMode="numeric"
                className="tabular-nums"
              />
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="scheme-reset-time" className="text-sm font-medium">
              {t('allocationResetTime')}
            </label>
            <Input
              id="scheme-reset-time"
              type="time"
              step={60}
              value={resetTime}
              onChange={(e) => setResetTime(e.target.value)}
              className="tabular-nums"
            />
          </div>
          <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">
            {t('allocationResetZoneHint', { zone: timeZone })}{' '}
            {period === 'month' && t('allocationResetShortMonthHint')}
          </p>
          {shareMode && (
            <p className="text-sm leading-6 text-muted-foreground sm:col-span-2">
              {t(
                ratioUnit === 'amount'
                  ? 'allocationTotalAmountHint'
                  : 'allocationTotalTokensHint',
              )}
            </p>
          )}
        </div>
        <section className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="font-medium">
              {t(shareMode ? 'allocationShares' : 'allocationMembers')}
            </h3>
            {shareMode && (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!members.length}
                onClick={() => {
                  setValues(
                    Object.fromEntries(
                      members.map((member, index) => [
                        member.id,
                        allocationValue(
                          Math.floor(10000 / members.length) +
                            (index < 10000 % members.length ? 1 : 0),
                          'ratio',
                        ),
                      ]),
                    ),
                  )
                }}
              >
                {t('allocationSplitEqually')}
              </Button>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            {t('allocationBlankHint')}
          </p>
          <div className="divide-y divide-border">
            {members.map((m) => (
              <div
                key={m.id}
                className="flex items-center justify-between gap-4 py-3"
              >
                <label
                  htmlFor={`share-${m.id}`}
                  className="min-w-0 break-words text-sm"
                >
                  {m.username}
                </label>
                <div className="flex shrink-0 items-center gap-2">
                  <Input
                    id={`share-${m.id}`}
                    aria-label={t('allocationFor', { name: m.username })}
                    value={values[m.id] ?? ''}
                    onChange={(e) =>
                      setValues((v) => ({ ...v, [m.id]: e.target.value }))
                    }
                    inputMode="decimal"
                    className="w-32 text-right tabular-nums"
                  />
                  <span className="w-9 text-sm text-muted-foreground">
                    {shareMode ? '%' : mode === 'amount' ? 'USD' : 'M'}
                  </span>
                </div>
              </div>
            ))}
          </div>
          {shareMode && (
            <p
              className={
                total > 10000
                  ? 'text-sm text-error'
                  : 'text-sm text-muted-foreground'
              }
            >
              {t('allocationTotal', {
                total: allocationValue(total, 'ratio'),
                remaining: allocationValue(Math.max(0, 10000 - total), 'ratio'),
              })}
            </p>
          )}
        </section>
        {shareMode && (
          <p className="text-sm leading-6 text-muted-foreground">
            {t(
              ratioUnit === 'amount'
                ? 'allocationRatioAmountRule'
                : 'allocationRatioTokenRule',
            )}
          </p>
        )}
        {scheme && (
          <p className="text-sm leading-6 text-muted-foreground">
            {t('allocationNextHint')}
          </p>
        )}
        <details
          className="space-y-4 border-t border-border pt-4"
          open={
            mode !== 'ratio' || !!scheme || advancedRates ? true : undefined
          }
        >
          <summary className="cursor-pointer text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring">
            {t('allocationSettings')}
          </summary>
          {shareMode && ratioUnit === 'amount' && !advancedRates && (
            <div className="space-y-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setAdvancedRates(true)}
              >
                {t('allocationAdvancedRates')}
              </Button>
            </div>
          )}
          {(mode === 'amount' ||
            (shareMode && ratioUnit === 'amount' && advancedRates)) && (
            <section className="space-y-3">
              <h3 className="font-medium">{t('allocationRates')}</h3>
              <p className="text-sm leading-6 text-muted-foreground">
                {t('allocationRatesHint')}
              </p>
              {rates.map((r, i) => (
                <div
                  key={i}
                  className="grid gap-3 border-b border-border pb-4 sm:grid-cols-3"
                >
                  <div className="space-y-2 sm:col-span-3">
                    <label htmlFor={`rate-model-${i}`} className="text-sm">
                      {t('allocationModelID')}
                    </label>
                    <div className="flex gap-2">
                      <ModelChoice
                        id={`rate-model-${i}`}
                        groupID={groupID}
                        value={r.model}
                        excluded={rates.map((rate) => rate.model)}
                        pending={pending}
                        onChange={(model) => fillPrice(i, model)}
                      />
                      <Button
                        variant="outline"
                        type="button"
                        onClick={() =>
                          setRates((all) => all.filter((_, n) => n !== i))
                        }
                        aria-label={t('allocationRemoveRate', { index: i + 1 })}
                      >
                        {t('allocationRemove')}
                      </Button>
                    </div>
                  </div>
                  {(['input', 'cached', 'output'] as const).map((field) => (
                    <div key={field} className="space-y-2">
                      <label htmlFor={`rate-${field}-${i}`} className="text-sm">
                        {t(
                          field === 'input'
                            ? 'allocationRateInput'
                            : field === 'cached'
                              ? 'allocationRateCached'
                              : 'allocationRateOutput',
                        )}
                      </label>
                      <Input
                        id={`rate-${field}-${i}`}
                        value={r[field]}
                        disabled={r.priceState === 'loading'}
                        inputMode="decimal"
                        onChange={(e) => updateRate(i, field, e.target.value)}
                      />
                    </div>
                  ))}
                  {r.priceState && (
                    <p className="text-xs text-muted-foreground sm:col-span-3">
                      {t(
                        r.priceState === 'loading'
                          ? 'allocationPriceLoading'
                          : r.priceState === 'missing'
                            ? 'allocationPriceMissing'
                            : 'allocationPriceFailed',
                      )}
                    </p>
                  )}
                </div>
              ))}
              <div className="flex flex-wrap items-start gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={rates.length >= 128}
                  onClick={() =>
                    setRates((all) => [
                      ...all,
                      { model: '', input: '', cached: '', output: '' },
                    ])
                  }
                >
                  {t('allocationAddRate')}
                </Button>
                <AddAllModels
                  groupID={groupID}
                  rates={rates}
                  pending={pending}
                  onAdd={fillAllPrices}
                />
              </div>
            </section>
          )}
          <div className="space-y-3 border-t border-border pt-5">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              {t('allocationEnabled')}
            </label>
            {!scheme && (
              <label className="flex items-start gap-2 text-sm leading-6">
                <input
                  type="checkbox"
                  className="mt-1 size-4 shrink-0 accent-primary"
                  checked={startNext}
                  onChange={(e) => setStartNext(e.target.checked)}
                />
                {t('allocationStartNext')}
              </label>
            )}
            {!scheme && !startNext && (
              <p className="text-sm leading-6 text-muted-foreground">
                {t('allocationImmediateHint')}
              </p>
            )}
          </div>
        </details>
      </fieldset>
      {invalid && (
        <p role="alert" className="text-sm text-error">
          {t('allocationInvalid')}
        </p>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={onCancel}
          disabled={pending}
        >
          {t('cancel')}
        </Button>
        <Button
          type="submit"
          disabled={
            pending ||
            (!tokenBased && rates.some((rate) => rate.priceState === 'loading'))
          }
        >
          {t('allocationSave')}
        </Button>
      </div>
    </form>
  )
}
