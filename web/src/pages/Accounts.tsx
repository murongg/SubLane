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
import { cn } from '@/lib/cn'
import { AccountUsage } from '@/components/AccountUsage'
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

const accountColumns =
  '@3xl:grid-cols-[minmax(0,1fr)_minmax(0,16rem)_minmax(8.5rem,0.7fr)_6rem]'

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
        <div className="@container overflow-hidden rounded-xl border border-border bg-card">
          <div
            aria-hidden="true"
            className={cn(
              'hidden gap-x-6 border-b border-border px-5 py-3 text-xs text-muted-foreground @3xl:grid',
              accountColumns,
            )}
          >
            <span>{t('accountName')}</span>
            <span>{t('accountUsage')}</span>
            <span>{t('memberStatus')}</span>
            <span className="text-right">{t('actions')}</span>
          </div>
          <ul
            className="divide-y divide-border"
            aria-label={t('accountsTitle')}
          >
            {query.data.accounts.map((account) => (
              <li key={account.id}>
                <article
                  aria-labelledby={`account-${account.id}`}
                  className={cn(
                    'grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-6 gap-y-5 p-5',
                    accountColumns,
                  )}
                >
                  <div className="col-span-2 min-w-0 @3xl:col-span-1">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5">
                      <h2
                        id={`account-${account.id}`}
                        className="break-words text-sm font-medium"
                      >
                        {account.name}
                      </h2>
                      {account.plan && (
                        <span className="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">
                          {account.plan}
                        </span>
                      )}
                    </div>
                    {account.email && (
                      <p className="mt-1.5 break-all text-xs leading-5 text-muted-foreground">
                        {account.email}
                      </p>
                    )}
                  </div>
                  <div className="col-span-2 min-w-0 @3xl:col-span-1">
                    {account.enabled && account.status !== 'reauth_required' ? (
                      <AccountUsage id={account.id} name={account.name} />
                    ) : (
                      <p className="text-xs leading-5 text-muted-foreground">
                        {t(
                          account.enabled
                            ? 'accountReauthorizeHint'
                            : 'accountDisabledHint',
                        )}
                      </p>
                    )}
                  </div>
                  <div className="min-w-0 space-y-2.5">
                    <AccountStatus account={account} />
                    <p className="text-xs leading-5 text-muted-foreground">
                      <span className="block">{t('accountExpires')}</span>
                      {account.expires_at ? (
                        <time
                          dateTime={new Date(
                            account.expires_at * 1000,
                          ).toISOString()}
                        >
                          {dates.format(account.expires_at * 1000)}
                        </time>
                      ) : (
                        t('accountUnknownExpiry')
                      )}
                    </p>
                  </div>
                  <div className="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground [@media(pointer:coarse)]:size-11"
                      title={t('checkAccount')}
                      aria-label={t('verifyAccountConnection', {
                        name: account.name,
                      })}
                      disabled={pending || !account.enabled}
                      onClick={() => {
                        clear()
                        check.mutate(account.id)
                      }}
                    >
                      {check.isPending && check.variables === account.id ? (
                        <LoaderCircle
                          className="size-4 motion-safe:animate-spin"
                          aria-hidden="true"
                        />
                      ) : (
                        <Check className="size-4" aria-hidden="true" />
                      )}
                    </Button>
                    <DropdownMenu modal={false}>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-8 text-muted-foreground [@media(pointer:coarse)]:size-11"
                          title={t('accountActions', { name: account.name })}
                          aria-label={t('accountActions', {
                            name: account.name,
                          })}
                          disabled={pending}
                        >
                          <MoreHorizontal
                            className="size-4"
                            aria-hidden="true"
                          />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
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
                </article>
              </li>
            ))}
          </ul>
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
