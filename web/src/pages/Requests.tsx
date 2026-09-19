import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, LoaderCircle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { accountOptions, providerLabels } from '@/lib/accounts'
import { requestOptions, outcomeKeys, outcomes } from '@/lib/requests'
import { reasonKeys } from '@/lib/runtime'
import { authKey, authOptions, type AuthState } from '@/lib/auth'
import { Button } from '@/components/ui/Button'
import { Status } from '@/components/Status'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/DropdownMenu'

export function Requests() {
  return <RequestHistory scope="personal" />
}

export function AllRequests() {
  return <RequestHistory scope="all" />
}

function RequestHistory({ scope }: { scope: 'personal' | 'all' }) {
  const { data: auth } = useQuery(authOptions())
  const user = auth?.user
  if (!user || (scope === 'all' && user.role !== 'admin')) return null
  return (
    <RequestTable key={`${scope}:${user.id}`} scope={scope} userID={user.id} />
  )
}

function RequestTable({
  scope,
  userID,
}: {
  scope: 'personal' | 'all'
  userID: number
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const personal = scope === 'personal'
  const [cursors, setCursors] = useState([0])
  const [account, setAccount] = useState('')
  const [outcome, setOutcome] = useState('')
  const query = useQuery(
    requestOptions(
      client,
      userID,
      scope,
      cursors[cursors.length - 1],
      account,
      outcome,
    ),
  )
  const accounts = useQuery({
    ...accountOptions,
    enabled: () =>
      !personal &&
      client.getQueryData<AuthState>(authKey)?.user?.role === 'admin',
  })
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'short',
  })
  const clock = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    timeStyle: 'medium',
  })
  const numbers = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en')
  const count = (value: number | null) =>
    value === null ? '—' : numbers.format(value)
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="page-title">
            {t(personal ? 'yourRequests' : 'allRequests')}
          </h1>
          <p className="page-description">
            {t(
              personal ? 'personalRequestsDescription' : 'requestsDescription',
            )}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={query.isFetching}
          onClick={() => query.refetch()}
        >
          <RefreshCw
            aria-hidden="true"
            className={query.isFetching ? 'motion-safe:animate-spin' : ''}
          />
          {t('refresh')}
        </Button>
      </div>
      <div className="flex flex-wrap gap-3">
        {!personal && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" aria-label={t('requestAccountFilter')}>
                {account
                  ? (accounts.data?.accounts.find((item) => item.id === account)
                      ?.name ?? t('requestDeletedAccount'))
                  : t('allRequestAccounts')}
                <ChevronDown aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuRadioGroup
                value={account}
                onValueChange={(value) => {
                  setAccount(value)
                  setCursors([0])
                }}
              >
                <DropdownMenuRadioItem value="">
                  {t('allRequestAccounts')}
                </DropdownMenuRadioItem>
                {accounts.data?.accounts.map((item) => (
                  <DropdownMenuRadioItem key={item.id} value={item.id}>
                    {item.name}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" aria-label={t('requestResultFilter')}>
              {outcome
                ? t(outcomeKeys[outcome as keyof typeof outcomeKeys])
                : t('allRequestOutcomes')}
              <ChevronDown aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuRadioGroup
              value={outcome}
              onValueChange={(value) => {
                setOutcome(value)
                setCursors([0])
              }}
            >
              <DropdownMenuRadioItem value="">
                {t('allRequestOutcomes')}
              </DropdownMenuRadioItem>
              {outcomes.map((value) => (
                <DropdownMenuRadioItem key={value} value={value}>
                  {t(outcomeKeys[value])}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 py-10 text-sm text-muted-foreground"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-4 motion-safe:animate-spin"
          />
          {t('loadingRequests')}
        </p>
      ) : query.isError ? (
        <div role="alert" className="space-y-3">
          <p className="text-sm text-error">{t('requestsLoadFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.requests.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border p-10 text-center">
          <h2 className="font-medium">{t('noRequests')}</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {t('noRequestsHint')}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border text-muted-foreground">
              <tr>
                {(
                  [
                    'requestTime',
                    personal ? 'requestKey' : 'requestCaller',
                    personal ? 'requestGroup' : 'requestAccount',
                    'requestModel',
                    'requestResult',
                    'requestDuration',
                    'requestTokens',
                  ] as const
                ).map((label) => (
                  <th
                    key={label}
                    scope="col"
                    className="whitespace-nowrap px-4 py-3 font-medium"
                  >
                    {t(label)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {query.data.requests.map((item) => (
                <tr key={item.id}>
                  <td className="whitespace-nowrap px-4 py-4 text-xs text-muted-foreground">
                    <time
                      dateTime={new Date(item.started_at * 1000).toISOString()}
                    >
                      <span className="block">
                        {dates.format(item.started_at * 1000)}
                      </span>
                      <span className="mt-1 block">
                        {clock.format(item.started_at * 1000)}
                      </span>
                    </time>
                  </td>
                  <td className="px-4 py-4">
                    {!personal && (
                      <p className="whitespace-nowrap">
                        {item.username || '—'}
                      </p>
                    )}
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.key_name || '—'}
                    </p>
                  </td>
                  <td className="px-4 py-4">
                    {!personal && (
                      <p className="min-w-24">
                        {item.account_name ||
                          (item.account_id ? t('requestDeletedAccount') : '—')}
                      </p>
                    )}
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.group_id === 1
                        ? t('defaultGroup')
                        : item.group_name || '—'}
                    </p>
                  </td>
                  <td className="px-4 py-4">
                    <code
                      className="block w-48 truncate text-xs"
                      title={item.model}
                    >
                      {item.model || '—'}
                    </code>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.provider && `${providerLabels[item.provider]} · `}
                      {item.transport === 'websocket'
                        ? 'WebSocket'
                        : 'HTTP'} · {item.operation}
                    </p>
                  </td>
                  <td className="px-4 py-4">
                    <Status
                      kind={
                        item.outcome === 'success'
                          ? 'success'
                          : item.outcome === 'canceled'
                            ? 'neutral'
                            : 'warning'
                      }
                    >
                      {t(outcomeKeys[item.outcome])}
                    </Status>
                    {item.error_code && (
                      <p className="mt-1 min-w-28 text-xs text-muted-foreground">
                        {t(reasonKeys[item.error_code] ?? 'reasonUnknown')}
                        {item.upstream_status !== null &&
                          ` · ${item.upstream_status}`}
                      </p>
                    )}
                  </td>
                  <td className="whitespace-nowrap px-4 py-4 tabular-nums">
                    {numbers.format(item.duration_ms)} ms
                  </td>
                  <td className="whitespace-nowrap px-4 py-4 text-xs tabular-nums">
                    {count(item.input_tokens)} / {count(item.output_tokens)}
                    {item.cached_tokens !== null && (
                      <p className="mt-1 text-muted-foreground">
                        {t('requestCachedTokens', {
                          count: numbers.format(item.cached_tokens),
                        })}
                      </p>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {query.data && (cursors.length > 1 || query.data.next_cursor !== 0) && (
        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            disabled={cursors.length === 1 || query.isFetching}
            onClick={() => setCursors((value) => value.slice(0, -1))}
          >
            {t('previousPage')}
          </Button>
          <Button
            variant="outline"
            disabled={!query.data.next_cursor || query.isFetching}
            onClick={() =>
              setCursors((value) => [...value, query.data.next_cursor])
            }
          >
            {t('nextPage')}
          </Button>
        </div>
      )}
    </div>
  )
}
