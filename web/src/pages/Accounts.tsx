import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  Power,
  RefreshCw,
  Trash2,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  accountErrorKey,
  accountOptions,
  checkAccount,
  setAccountEnabled,
  type Account,
} from '@/lib/accounts'
import { ConnectAccount } from '@/components/ConnectAccount'
import { DeleteAccount } from '@/components/DeleteAccount'
import { Status } from '@/components/Status'
import { Button } from '@/components/ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/DropdownMenu'

export function Accounts() {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(accountOptions)
  const [connecting, setConnecting] = useState<Account | 'new' | null>(null)
  const [removing, setRemoving] = useState<Account | null>(null)
  const [verified, setVerified] = useState<{
    name: string
    count: number
  } | null>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const invalidate = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['accounts'] }),
      client.invalidateQueries({ queryKey: ['system'] }),
      client.invalidateQueries({ queryKey: ['connection'] }),
    ])
  }
  const update = useMutation({
    mutationFn: setAccountEnabled,
    onSuccess: invalidate,
  })
  const check = useMutation({
    mutationFn: checkAccount,
    onSuccess: async (value) => {
      setVerified({ name: value.account.name, count: value.models.length })
      await invalidate()
    },
    onError: () => client.invalidateQueries({ queryKey: ['accounts'] }),
  })
  const pending = update.isPending || check.isPending
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  const restoreFocus = () => heading.current?.focus()
  const clear = () => {
    setVerified(null)
    update.reset()
    check.reset()
  }
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 ref={heading} tabIndex={-1} className="page-title outline-none">
            {t('accountsTitle')}
          </h1>
          <p className="page-description">{t('accountsDescription')}</p>
        </div>
        <Button
          onClick={() => {
            clear()
            setConnecting('new')
          }}
        >
          <Plus aria-hidden="true" />
          {t('addAccount')}
        </Button>
      </div>
      {verified && (
        <p role="status" className="text-sm text-success">
          {t('accountVerificationResult', verified)}
        </p>
      )}
      {(update.error || check.error) && (
        <p role="alert" className="text-sm text-error">
          {t(accountErrorKey(update.error ?? check.error))}
        </p>
      )}
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 py-10 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loadingAccounts')}
        </p>
      ) : query.isError ? (
        <div
          role="alert"
          className="space-y-4 rounded-xl border border-border p-6"
        >
          <p className="text-sm text-error">{t('accountsLoadFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.accounts.length === 0 ? (
        <section className="flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-14 text-center">
          <Workflow
            className="mb-4 size-8 text-muted-foreground"
            aria-hidden="true"
          />
          <h2 className="font-medium">{t('accountEmptyTitle')}</h2>
          <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
            {t('accountEmptyDescription')}
          </p>
        </section>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border text-muted-foreground">
              <tr>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('accountName')}
                </th>
                <th
                  scope="col"
                  className="hidden px-5 py-3 font-medium sm:table-cell"
                >
                  {t('memberStatus')}
                </th>
                <th
                  scope="col"
                  className="hidden px-5 py-3 font-medium xl:table-cell"
                >
                  {t('accountExpires')}
                </th>
                <th scope="col" className="px-5 py-3 text-right font-medium">
                  {t('actions')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {query.data.accounts.map((account) => (
                <tr key={account.id}>
                  <td className="min-w-40 max-w-72 px-5 py-4">
                    <p className="break-words font-medium">{account.name}</p>
                    {account.email && (
                      <p className="mt-1 break-all text-xs text-muted-foreground">
                        {account.email}
                      </p>
                    )}
                    {account.plan && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {account.plan}
                      </p>
                    )}
                    <div className="mt-2 sm:hidden">
                      <AccountStatus account={account} />
                    </div>
                    <p className="mt-2 text-xs leading-5 text-muted-foreground xl:hidden">
                      {t('accountExpires')}:{' '}
                      {account.expires_at
                        ? dates.format(account.expires_at * 1000)
                        : t('accountUnknownExpiry')}
                    </p>
                  </td>
                  <td className="hidden px-5 py-4 sm:table-cell">
                    <AccountStatus account={account} />
                  </td>
                  <td className="hidden whitespace-nowrap px-5 py-4 text-muted-foreground xl:table-cell">
                    {account.expires_at
                      ? dates.format(account.expires_at * 1000)
                      : t('accountUnknownExpiry')}
                  </td>
                  <td className="px-5 py-4">
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        className="hidden lg:inline-flex"
                        disabled={pending || !account.enabled}
                        onClick={() => {
                          clear()
                          check.mutate(account.id)
                        }}
                      >
                        {check.isPending && check.variables === account.id ? (
                          <LoaderCircle
                            className="motion-safe:animate-spin"
                            aria-hidden="true"
                          />
                        ) : (
                          <Check aria-hidden="true" />
                        )}
                        {t('checkAccount')}
                      </Button>
                      <DropdownMenu modal={false}>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="[@media(pointer:coarse)]:size-11"
                            aria-label={t('accountActions', {
                              name: account.name,
                            })}
                            disabled={pending}
                          >
                            <MoreHorizontal aria-hidden="true" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            className="lg:hidden [@media(pointer:coarse)]:min-h-11"
                            disabled={!account.enabled}
                            onSelect={() => {
                              clear()
                              check.mutate(account.id)
                            }}
                          >
                            <Check aria-hidden="true" />
                            {t('checkAccount')}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="[@media(pointer:coarse)]:min-h-11"
                            onSelect={() => {
                              clear()
                              setConnecting(account)
                            }}
                          >
                            <RefreshCw aria-hidden="true" />
                            {t('reauthorizeAccount')}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="[@media(pointer:coarse)]:min-h-11"
                            onSelect={() => {
                              clear()
                              update.mutate({
                                id: account.id,
                                enabled: !account.enabled,
                              })
                            }}
                          >
                            <Power aria-hidden="true" />
                            {t(
                              account.enabled
                                ? 'disableAccount'
                                : 'enableAccount',
                            )}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-error focus:bg-error-muted focus:text-error [@media(pointer:coarse)]:min-h-11"
                            onSelect={() => {
                              clear()
                              setRemoving(account)
                            }}
                          >
                            <Trash2 aria-hidden="true" />
                            {t('deleteAccount')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {connecting && (
        <ConnectAccount
          account={connecting === 'new' ? undefined : connecting}
          onClose={() => setConnecting(null)}
          onCreated={invalidate}
          restoreFocus={restoreFocus}
        />
      )}
      {removing && (
        <DeleteAccount
          account={removing}
          onClose={() => setRemoving(null)}
          onDeleted={invalidate}
          restoreFocus={restoreFocus}
        />
      )}
    </div>
  )
}

function AccountStatus({ account }: { account: Account }) {
  const { t } = useTranslation()
  return (
    <Status
      kind={
        !account.enabled
          ? 'neutral'
          : account.status === 'ready'
            ? 'success'
            : account.status === 'reauth_required'
              ? 'error'
              : 'warning'
      }
    >
      {t(
        !account.enabled
          ? 'disabled'
          : account.status === 'ready'
            ? 'accountVerified'
            : account.status === 'reauth_required'
              ? 'accountNeedsAuth'
              : 'accountUnverified',
      )}
    </Status>
  )
}
