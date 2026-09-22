import { useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  budgetErrorKey,
  parseBudgetMillions,
  saveBudget,
  type Budget,
} from '@/lib/budgets'
import { groupOptions } from '@/lib/groups'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

export function BudgetForm({
  userID,
  rule,
  onSaved,
  onCancel,
  onBusy,
}: {
  userID: number
  rule: Budget | null
  onSaved: () => Promise<unknown>
  onCancel: () => void
  onBusy: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const groups = useQuery(groupOptions)
  const [groupID, setGroupID] = useState(rule?.group_id ?? 0)
  const [period, setPeriod] = useState<'day' | 'month'>(rule?.period ?? 'day')
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: saveBudget,
    onSuccess: onSaved,
    onSettled: () => onBusy(false),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (mutation.isPending) return
    const data = new FormData(event.currentTarget)
    const model = rule?.model ?? String(data.get('model') ?? '').trim()
    const limit = parseBudgetMillions(String(data.get('limit') ?? ''))
    const bad =
      limit === null ||
      limit < 1 ||
      limit > 1_000_000_000_000 ||
      (model !== '' && !/^[a-zA-Z0-9][a-zA-Z0-9._:/()+-]{0,127}$/.test(model))
    setInvalid(bad)
    if (bad || limit === null) return
    onBusy(true)
    mutation.mutate({
      userID,
      input: { group_id: groupID, model, period, limit, enabled },
    })
  }
  const groupName =
    groupID === 0
      ? t('budgetAllGroups')
      : groupID === 1
        ? t('defaultGroup')
        : (groups.data?.groups.find((group) => group.id === groupID)?.name ??
          rule?.group_name)
  return (
    <form onSubmit={submit} className="space-y-4 border-t border-border pt-4">
      <h3 className="font-medium">{t(rule ? 'budgetEdit' : 'budgetAdd')}</h3>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label id={id + '-group'} className="text-sm font-medium">
            {t('budgetGroup')}
          </label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="outline"
                className="w-full justify-between"
                aria-labelledby={id + '-group'}
                disabled={
                  !!rule ||
                  mutation.isPending ||
                  groups.isPending ||
                  groups.isError
                }
              >
                {groupName}
                <ChevronDown aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={String(groupID)}
                onValueChange={(value) => setGroupID(Number(value))}
              >
                <DropdownMenuRadioItem value="0">
                  {t('budgetAllGroups')}
                </DropdownMenuRadioItem>
                {groups.data?.groups.map((group) => (
                  <DropdownMenuRadioItem
                    key={group.id}
                    value={String(group.id)}
                  >
                    {group.id === 1 ? t('defaultGroup') : group.name}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="space-y-2">
          <label id={id + '-period'} className="text-sm font-medium">
            {t('budgetPeriod')}
          </label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="outline"
                className="w-full justify-between"
                aria-labelledby={id + '-period'}
                disabled={!!rule || mutation.isPending}
              >
                {t(period === 'day' ? 'budgetDaily' : 'budgetMonthly')}
                <ChevronDown aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={period}
                onValueChange={(value) => setPeriod(value as 'day' | 'month')}
              >
                <DropdownMenuRadioItem value="day">
                  {t('budgetDaily')}
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="month">
                  {t('budgetMonthly')}
                </DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="space-y-2">
          <label htmlFor={id + '-model'} className="text-sm font-medium">
            {t('budgetModel')}
          </label>
          <Input
            id={id + '-model'}
            name="model"
            autoFocus={!rule}
            defaultValue={rule?.model ?? ''}
            maxLength={128}
            disabled={!!rule || mutation.isPending}
            aria-describedby={id + '-hint'}
          />
          <p id={id + '-hint'} className="text-xs text-muted-foreground">
            {t('budgetModelHint')}
          </p>
        </div>
        <div className="space-y-2">
          <label htmlFor={id + '-limit'} className="text-sm font-medium">
            {t('budgetLimit')}
          </label>
          <Input
            aria-describedby={id + '-unit'}
            id={id + '-limit'}
            name="limit"
            autoFocus={!!rule}
            type="number"
            min={0.000001}
            max={1_000_000}
            step={0.000001}
            required
            defaultValue={rule ? rule.limit / 1_000_000 : ''}
            disabled={mutation.isPending}
          />
        </div>
      </div>
      {groups.isError && (
        <div role="alert" className="space-y-2 text-sm">
          <p className="text-error">{t('groupsLoadFailed')}</p>
          <Button
            type="button"
            variant="outline"
            onClick={() => groups.refetch()}
          >
            {t('reconnect')}
          </Button>
        </div>
      )}
      <p id={id + '-unit'} className="text-xs text-muted-foreground">
        {t('budgetUnitHint')}
      </p>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={enabled}
          disabled={mutation.isPending}
          onChange={(event) => setEnabled(event.target.checked)}
          className="size-4 accent-primary"
        />
        {t('budgetEnabled')}
      </label>
      <p className="text-xs leading-5 text-muted-foreground">
        {t('budgetEditHint')}
      </p>
      {invalid && (
        <p role="alert" className="text-sm text-error">
          {t('budgetInvalid')}
        </p>
      )}
      {mutation.isError && (
        <p role="alert" className="text-sm text-error">
          {t(budgetErrorKey(mutation.error))}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={mutation.isPending}
          onClick={onCancel}
        >
          {t('cancel')}
        </Button>
        <Button type="submit" disabled={mutation.isPending}>
          {t(mutation.isPending ? 'budgetSaving' : 'budgetSave')}
        </Button>
      </div>
    </form>
  )
}
