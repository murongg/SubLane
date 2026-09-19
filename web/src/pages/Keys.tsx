import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authOptions } from '@/lib/auth'
import { keyOptions } from '@/lib/keys'
import { CreateKey } from '@/components/CreateKey'
import { ClientGuide } from '@/components/ClientGuide'
import { RevokeKey } from '@/components/RevokeKey'
import { Status } from '@/components/Status'
import { Button } from '@/components/ui/Button'

export function Keys() {
  const { data } = useQuery(authOptions())
  if (!data?.user) return null
  // A cross-tab identity change must unmount dialogs as well as clear query data, including any displayed secret.
  return <KeyManager key={data.user.id} userID={data.user.id} />
}

function KeyManager({ userID }: { userID: number }) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const heading = useRef<HTMLHeadingElement>(null)
  const [cursors, setCursors] = useState([0])
  const query = useQuery(
    keyOptions(client, userID, cursors[cursors.length - 1]),
  )
  const invalidate = () => {
    return client.invalidateQueries({ queryKey: ['keys', userID] })
  }
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    dateStyle: 'medium',
  })
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 ref={heading} tabIndex={-1} className="page-title outline-none">
            {t('apiKeys')}
          </h1>
          <p className="page-description">{t('apiKeysDescription')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ClientGuide userID={userID} />
          <CreateKey
            userID={userID}
            onCreated={() => {
              setCursors([0])
              return invalidate()
            }}
          />
        </div>
      </div>
      <p className="text-sm leading-6 text-muted-foreground">
        {t('gatewayKeysNotice')}
      </p>
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 py-10 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loadingKeys')}
        </p>
      ) : query.isError ? (
        <div
          role="alert"
          className="space-y-4 rounded-xl border border-border p-6"
        >
          <p className="text-sm text-error">{t('keysLoadFailed')}</p>
          <Button
            variant="outline"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.keys.length === 0 ? (
        <section className="flex flex-col items-center rounded-xl border border-dashed border-border p-10 text-center">
          <KeyRound
            className="mb-4 size-8 text-muted-foreground"
            aria-hidden="true"
          />
          <h2 className="font-medium">{t('noKeys')}</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {t('noKeysDescription')}
          </p>
        </section>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border text-muted-foreground">
              <tr>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('keyName')}
                </th>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('memberStatus')}
                </th>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('keyLastUsed')}
                </th>
                <th scope="col" className="px-5 py-3 text-right font-medium">
                  {t('actions')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {query.data.keys.map((key) => (
                <tr key={key.id}>
                  <td className="max-w-64 px-5 py-4">
                    <p className="break-words font-medium">{key.name}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('keyGroupName', {
                        name:
                          key.group_id === 1
                            ? t('defaultGroup')
                            : key.group_name,
                      })}
                    </p>
                    <code className="mt-1 block text-xs text-muted-foreground">
                      {key.prefix}…
                    </code>
                  </td>
                  <td className="px-5 py-4">
                    <Status
                      kind={
                        key.revoked_at !== null
                          ? 'neutral'
                          : key.group_access === 'allowed'
                            ? 'success'
                            : 'warning'
                      }
                    >
                      {t(
                        key.revoked_at !== null
                          ? 'revoked'
                          : key.group_access === 'allowed'
                            ? 'active'
                            : 'keyGroupUnavailable',
                      )}
                    </Status>
                  </td>
                  <td className="whitespace-nowrap px-5 py-4 text-muted-foreground">
                    {key.last_used_at === null
                      ? t('neverUsed')
                      : dates.format(key.last_used_at * 1000)}
                  </td>
                  <td className="px-5 py-4 text-right">
                    <RevokeKey
                      value={key}
                      onRevoked={invalidate}
                      focusAfterRevoke={() => heading.current?.focus()}
                    />
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
