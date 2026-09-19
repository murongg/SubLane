import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { accountOptions, providerLabels } from '@/lib/accounts'
import { authKey, type AuthState } from '@/lib/auth'
import {
  groupDetails,
  groupErrorKey,
  saveGroup,
  type GroupDetail,
} from '@/lib/groups'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

type Props = { id?: number; onClose: () => void; onSaved: () => Promise<void> }
export function GroupEditor({ id, onClose, onSaved }: Props) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const previousFocus = useRef(document.activeElement as HTMLElement | null)
  const restoreFocus = () => previousFocus.current?.focus()
  const query = useQuery({
    queryKey: ['group', id],
    queryFn: ({ signal }) => groupDetails(id!, signal),
    enabled: () =>
      Boolean(id) &&
      client.getQueryData<AuthState>(authKey)?.user?.role === 'admin',
    staleTime: 0,
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      {!id || (query.data && !query.isFetching && !query.isError) ? (
        <GroupForm
          value={query.data}
          onClose={onClose}
          onSaved={onSaved}
          restoreFocus={restoreFocus}
        />
      ) : (
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('editGroup')}</DialogTitle>
            <DialogDescription>{t('groupEditorDescription')}</DialogDescription>
          </DialogHeader>
          {query.isError ? (
            <div role="alert" className="space-y-3">
              <p className="text-sm text-error">{t('groupsLoadFailed')}</p>
              <Button variant="outline" onClick={() => query.refetch()}>
                {t('reconnect')}
              </Button>
            </div>
          ) : (
            <p role="status" className="text-sm text-muted-foreground">
              {t('loadingGroups')}
            </p>
          )}
        </DialogContent>
      )}
    </Dialog>
  )
}
function GroupForm({
  value,
  onClose,
  onSaved,
  restoreFocus,
}: {
  value?: GroupDetail
  onClose: () => void
  onSaved: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(value?.name ?? '')
  const [enabled, setEnabled] = useState(value?.enabled ?? true)
  const [selected, setSelected] = useState<string[]>(value?.account_ids ?? [])
  const [invalid, setInvalid] = useState(false)
  const accounts = useQuery(accountOptions)
  const mutation = useMutation({ mutationFn: saveGroup, onSuccess: onSaved })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending || !accounts.data) return
    const valid = name.trim().length > 0 && Array.from(name.trim()).length <= 64
    setInvalid(!valid)
    if (valid)
      mutation.mutate({
        id: value?.id,
        name: name.trim(),
        enabled,
        account_ids: selected,
      })
  }
  return (
    <DialogContent
      onCloseAutoFocus={(event) => {
        event.preventDefault()
        restoreFocus()
      }}
      showCloseButton={false}
      className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      onInteractOutside={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
      onEscapeKeyDown={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
    >
      <DialogHeader>
        <DialogTitle>{t(value ? 'editGroup' : 'createGroup')}</DialogTitle>
        <DialogDescription>{t('groupEditorDescription')}</DialogDescription>
      </DialogHeader>
      <form className="space-y-5" onSubmit={submit} noValidate>
        <div className="space-y-2">
          <label htmlFor="group-name" className="text-sm font-medium">
            {t('groupName')}
          </label>
          <Input
            id="group-name"
            value={name}
            maxLength={128}
            onChange={(event) => setName(event.target.value)}
            disabled={mutation.isPending || value?.is_default}
            aria-invalid={invalid}
          />
          {invalid && (
            <p role="alert" className="text-sm text-error">
              {t('groupInputInvalid')}
            </p>
          )}
        </div>
        <label className="flex items-center gap-3 text-sm">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            disabled={mutation.isPending || value?.is_default}
            className="size-4 accent-primary focus-visible:ring-2 focus-visible:ring-ring"
          />
          {t('groupEnabled')}
        </label>
        {value?.is_default && (
          <p className="text-xs leading-5 text-muted-foreground">
            {t('defaultGroupProtected')}
          </p>
        )}
        <fieldset className="space-y-2" disabled={mutation.isPending}>
          <legend className="text-sm font-medium">{t('groupAccounts')}</legend>
          <p className="text-xs leading-5 text-muted-foreground">
            {t('groupAccountsHint')}
          </p>
          {accounts.isPending ? (
            <p role="status" className="py-3 text-sm text-muted-foreground">
              {t('loadingAccounts')}
            </p>
          ) : accounts.isError ? (
            <div role="alert" className="space-y-2">
              <p className="text-sm text-error">{t('accountsLoadFailed')}</p>
              <Button
                type="button"
                variant="outline"
                onClick={() => accounts.refetch()}
              >
                {t('reconnect')}
              </Button>
            </div>
          ) : accounts.data.accounts.length === 0 ? (
            <p className="py-3 text-sm text-muted-foreground">
              {t('groupNoAccounts')}
            </p>
          ) : (
            <div className="max-h-60 divide-y divide-border overflow-y-auto rounded-lg border border-border">
              {accounts.data.accounts.map((account) => (
                <label
                  key={account.id}
                  className="flex cursor-pointer items-start gap-3 px-3 py-3 hover:bg-muted"
                >
                  <input
                    type="checkbox"
                    checked={selected.includes(account.id)}
                    onChange={(event) =>
                      setSelected((ids) =>
                        event.target.checked
                          ? [...ids, account.id]
                          : ids.filter((id) => id !== account.id),
                      )
                    }
                    className="mt-0.5 size-4 shrink-0 accent-primary focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <span className="min-w-0 text-sm">
                    <span className="block break-words font-medium">
                      {account.name}
                    </span>
                    <span className="mt-0.5 block break-words text-xs text-muted-foreground">
                      {providerLabels[account.provider]} · {account.email}
                      {!account.enabled && ` · ${t('disabled')}`}
                    </span>
                  </span>
                </label>
              ))}
            </div>
          )}
        </fieldset>
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t(groupErrorKey(mutation.error))}
          </p>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            {t('cancel')}
          </Button>
          <Button type="submit" disabled={mutation.isPending || !accounts.data}>
            {mutation.isPending && (
              <LoaderCircle
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            )}
            {t('saveGroup')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
