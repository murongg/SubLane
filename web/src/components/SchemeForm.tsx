import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'
import {
  type Team,
  type Scheme,
  type SchemeInput,
  type AllocationMode,
  modeLabels,
  parseAllocationValue,
  allocationValue,
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
  onChange,
}: {
  id: string
  label: string
  value: string
  options: { value: string; label: string }[]
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          className="w-full justify-between gap-3"
          aria-label={label}
          disabled={disabled}
        >
          <span className="truncate">
            {options.find((option) => option.value === value)?.label}
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
type RateDraft = {
  model: string
  input: string
  cached: string
  output: string
}
export function SchemeForm({
  teams,
  groups,
  scheme,
  onSubmit,
  onCancel,
  pending,
  fixedTeam,
  resourceMode = false,
}: {
  teams: Team[]
  groups: { id: number; name: string; enabled: boolean }[]
  scheme?: Scheme
  onSubmit: (input: SchemeInput) => void
  onCancel: () => void
  pending: boolean
  /** When set, the resource belongs to this team and the team cannot change. */
  fixedTeam?: Team
  /** Team resources use a generated name so the form only asks for policy inputs. */
  resourceMode?: boolean
}) {
  const { t } = useTranslation()
  const initial = scheme?.next?.config ?? scheme?.config
  const [name, setName] = useState(scheme?.name ?? '')
  const [teamID, setTeam] = useState(
    scheme?.team_id ?? fixedTeam?.id ?? teams[0]?.id ?? 0,
  )
  const [groupID, setGroup] = useState(scheme?.group_id ?? groups[0]?.id ?? 0)
  const [mode, setMode] = useState<AllocationMode>(initial?.mode ?? 'ratio')
  const [period, setPeriod] = useState(
    initial?.period === 'day' ? 'day' : 'month',
  )
  const [enabled, setEnabled] = useState(scheme?.enabled ?? true)
  const [startNext, setStartNext] = useState(true)
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
  const [invalid, setInvalid] = useState(false)
  const team = fixedTeam ?? teams.find((v) => v.id === teamID)
  const group = groups.find((v) => v.id === groupID)
  const total =
    team?.members.reduce(
      (sum, m) => sum + (parseAllocationValue(values[m.id] ?? '', mode) ?? 0),
      0,
    ) ?? 0
  const changeMode = (next: AllocationMode) => {
    if (next !== mode) {
      setMode(next)
      setValues({})
      setInvalid(false)
    }
  }
  const updateRate = (i: number, field: keyof RateDraft, value: string) =>
    setRates((all) =>
      all.map((r, n) => (n === i ? { ...r, [field]: value } : r)),
    )
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (pending) return
    const members = (team?.members ?? [])
      .filter((m) => (values[m.id] ?? '').trim() !== '')
      .map((m) => ({
        user_id: m.id,
        limit: parseAllocationValue(values[m.id], mode) ?? -1,
      }))
    const parsedRates =
      mode === 'tokens'
        ? []
        : rates.map((r) => ({
            model: r.model.trim(),
            input: parseAllocationValue(r.input, 'amount') ?? -1,
            cached: parseAllocationValue(r.cached, 'amount') ?? -1,
            output: parseAllocationValue(r.output, 'amount') ?? -1,
          }))
    const bad =
      (!resourceMode && !name.trim()) ||
      !teamID ||
      !groupID ||
      members.length === 0 ||
      members.some((m) => m.limit <= 0) ||
      (mode === 'ratio' && total > 10000) ||
      (mode !== 'tokens' &&
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
      name: resourceMode
        ? `${team?.name ?? ''} / ${group?.name ?? ''}`.trim()
        : name.trim(),
      team_id: teamID,
      group_id: groupID,
      enabled,
      start_next: startNext,
      config: {
        mode,
        period: mode === 'ratio' ? 'upstream' : (period as 'day' | 'month'),
        members,
        rates: parsedRates,
      },
    })
  }
  return (
    <form onSubmit={submit} className="space-y-6" noValidate>
      <fieldset disabled={pending} className="space-y-6">
        <div className="grid gap-4 sm:grid-cols-2">
          {!resourceMode && (
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
          )}
          {fixedTeam ? (
            <div className="space-y-2 sm:col-span-2">
              <span className="text-sm font-medium">{t('personnelTeam')}</span>
              <p className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
                {fixedTeam.name}
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              <label htmlFor="scheme-team" className="text-sm font-medium">
                {t('personnelTeam')}
              </label>
              <SchemeChoice
                id="scheme-team"
                label={t('personnelTeam')}
                value={String(teamID)}
                disabled={!!scheme || pending}
                options={teams.map((team) => ({
                  value: String(team.id),
                  label: team.name,
                }))}
                onChange={(value) => {
                  setTeam(Number(value))
                  setValues({})
                }}
              />
            </div>
          )}
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
                label: pool.id === 1 ? t('defaultGroup') : pool.name,
              }))}
              onChange={(value) => setGroup(Number(value))}
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
        {mode === 'ratio' ? (
          <p className="text-sm">{t('allocationUpstreamReset')}</p>
        ) : (
          <div className="max-w-xs space-y-2">
            <label htmlFor="scheme-period" className="text-sm font-medium">
              {t('allocationPeriod')}
            </label>
            <SchemeChoice
              id="scheme-period"
              label={t('allocationPeriod')}
              value={period}
              disabled={pending}
              options={[
                { value: 'day', label: t('budgetDaily') },
                { value: 'month', label: t('budgetMonthly') },
              ]}
              onChange={setPeriod}
            />
          </div>
        )}
        <section className="space-y-3">
          <h3 className="font-medium">{t('allocationMembers')}</h3>
          <p className="text-sm text-muted-foreground">
            {t('allocationBlankHint')}
          </p>
          <div className="divide-y divide-border">
            {team?.members.map((m) => (
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
                    {mode === 'ratio' ? '%' : mode === 'amount' ? 'USD' : 'M'}
                  </span>
                </div>
              </div>
            ))}
          </div>
          {mode === 'ratio' && (
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
        {mode !== 'tokens' && (
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
                    <Input
                      id={`rate-model-${i}`}
                      value={r.model}
                      onChange={(e) => updateRate(i, 'model', e.target.value)}
                      maxLength={128}
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
                      inputMode="decimal"
                      onChange={(e) => updateRate(i, field, e.target.value)}
                    />
                  </div>
                ))}
              </div>
            ))}
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
          {scheme ? (
            <p className="text-sm leading-6 text-muted-foreground">
              {t('allocationNextHint')}
            </p>
          ) : (
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
        <Button type="submit" disabled={pending}>
          {t('allocationSave')}
        </Button>
      </div>
    </form>
  )
}
