import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { requestCallerOptions, type RequestFilters } from '@/lib/requests'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from './ui/DropdownMenu'

function localDate(value: number | undefined) {
  if (!value) return ''
  const date = new Date(value * 1000)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}

export function RequestFilterForm({
  id,
  value,
  scope,
  userID,
  onApply,
}: {
  id: string
  value: RequestFilters
  scope: 'personal' | 'all'
  userID: number
  onApply: (value: RequestFilters) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const callers = useQuery(requestCallerOptions(client, userID, scope))
  const [draft, setDraft] = useState(value)
  const [from, setFrom] = useState(localDate(value.from))
  const [until, setUntil] = useState(localDate(value.until))
  const [error, setError] = useState(false)
  const users = [
    ...new Map(
      (callers.data?.callers ?? []).map((item) => [
        item.user_id,
        { id: String(item.user_id), name: item.username || `#${item.user_id}` },
      ]),
    ).values(),
  ]
  const keys = [
    ...new Map(
      (callers.data?.callers ?? [])
        .filter(
          (item) =>
            item.key_id > 0 &&
            (!draft.user_id ||
              scope === 'personal' ||
              item.user_id === draft.user_id),
        )
        .map((item) => [
          item.key_id,
          {
            id: String(item.key_id),
            name: `${item.key_name || `#${item.key_id}`}${scope === 'all' ? ` · ${item.username || `#${item.user_id}`}` : ''}`,
          },
        ]),
    ).values(),
  ]
  return (
    <form
      id={id}
      className="space-y-4 rounded-lg border border-border bg-card p-4"
      onSubmit={(event) => {
        event.preventDefault()
        const start = from
          ? Math.floor(new Date(from).getTime() / 1000)
          : undefined
        const end = until
          ? Math.floor(new Date(until).getTime() / 1000)
          : undefined
        if (
          (start !== undefined && !Number.isFinite(start)) ||
          (end !== undefined && !Number.isFinite(end)) ||
          (start !== undefined && end !== undefined && start >= end)
        ) {
          setError(true)
          return
        }
        setError(false)
        onApply({
          ...draft,
          model: draft.model?.trim(),
          request_id: draft.request_id?.trim(),
          from: start,
          until: end,
        })
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <label className="min-w-0 space-y-1.5 text-sm">
          <span>{t('requestModelFilter')}</span>
          <Input
            maxLength={160}
            value={draft.model ?? ''}
            onChange={(e) => setDraft({ ...draft, model: e.target.value })}
            placeholder={t('requestExactModel')}
          />
        </label>
        <label className="min-w-0 space-y-1.5 text-sm">
          <span>{t('requestID')}</span>
          <Input
            maxLength={64}
            value={draft.request_id ?? ''}
            onChange={(e) => setDraft({ ...draft, request_id: e.target.value })}
            placeholder="req_…"
          />
        </label>
        <div className="min-w-0 space-y-1.5 text-sm">
          <p>{t('requestKey')}</p>
          <CallerChoice
            label={t('requestKeyFilter')}
            all={t('requestAllKeys')}
            value={String(draft.key_id ?? '')}
            options={keys}
            disabled={callers.isPending}
            onChange={(key) =>
              setDraft({ ...draft, key_id: key ? Number(key) : undefined })
            }
          />
        </div>
        {scope === 'all' && (
          <div className="min-w-0 space-y-1.5 text-sm">
            <p>{t('requestMember')}</p>
            <CallerChoice
              label={t('requestMemberFilter')}
              all={t('requestAllMembers')}
              value={String(draft.user_id ?? '')}
              options={users}
              disabled={callers.isPending}
              onChange={(user) =>
                setDraft({
                  ...draft,
                  user_id: user ? Number(user) : undefined,
                  key_id: undefined,
                })
              }
            />
          </div>
        )}
        <label className="min-w-0 space-y-1.5 text-sm">
          <span>{t('requestFrom')}</span>
          <Input
            type="datetime-local"
            value={from}
            onChange={(e) => {
              setFrom(e.target.value)
              setError(false)
            }}
          />
        </label>
        <label className="min-w-0 space-y-1.5 text-sm">
          <span>{t('requestUntil')}</span>
          <Input
            type="datetime-local"
            value={until}
            onChange={(e) => {
              setUntil(e.target.value)
              setError(false)
            }}
          />
        </label>
      </div>
      {error && (
        <p role="alert" className="text-sm text-error">
          {t('requestInvalidRange')}
        </p>
      )}
      {callers.isError && (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2 text-sm text-error"
        >
          {t('requestFiltersFailed')}
          <Button
            type="button"
            variant="ghost"
            onClick={() => callers.refetch()}
          >
            {t('reconnect')}
          </Button>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {t('requestFilterHint')}
        </p>
        <Button type="submit">{t('requestApplyFilters')}</Button>
      </div>
    </form>
  )
}

function CallerChoice({
  label,
  all,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string
  all: string
  value: string
  options: { id: string; name: string }[]
  onChange: (value: string) => void
  disabled: boolean
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="w-full justify-between"
          aria-label={label}
          disabled={disabled}
        >
          <span className="truncate">
            {value
              ? (options.find((item) => item.id === value)?.name ?? `#${value}`)
              : all}
          </span>
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-64 max-w-[calc(100vw-2rem)] overflow-y-auto">
        <DropdownMenuRadioGroup value={value} onValueChange={onChange}>
          <DropdownMenuRadioItem value="">{all}</DropdownMenuRadioItem>
          {options.map((item) => (
            <DropdownMenuRadioItem key={item.id} value={item.id}>
              {item.name}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
