import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, Plus, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getMembers, setMemberEnabled } from '@/lib/members'
import { Button } from '@/components/ui/Button'
import { Status } from '@/components/Status'
import { CreateMember } from '@/components/CreateMember'

export function Members() {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const [cursors, setCursors] = useState([0])
  const cursor = cursors[cursors.length - 1]
  const [creating, setCreating] = useState(false)
  const [createdName, setCreatedName] = useState('')
  const query = useQuery({
    queryKey: ['members', cursor],
    queryFn: ({ signal }) => getMembers(cursor, signal),
  })
  const update = useMutation({
    mutationFn: setMemberEnabled,
    onSuccess: () => client.invalidateQueries({ queryKey: ['members'] }),
  })
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="page-title">{t('members')}</h1>
          <p className="page-description">{t('membersDescription')}</p>
        </div>
        <Button
          onClick={() => {
            setCreatedName('')
            setCreating(true)
          }}
        >
          <Plus aria-hidden="true" />
          {t('addMember')}
        </Button>
      </div>
      {createdName && (
        <p role="status" className="text-sm text-success">
          {t('memberCreated', { username: createdName })}
        </p>
      )}
      {update.isError && (
        <p role="alert" className="text-sm text-error">
          {t('memberUpdateFailed')}
        </p>
      )}
      {query.isPending ? (
        <div
          role="status"
          className="flex items-center gap-2 py-10 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loadingMembers')}
        </div>
      ) : query.isError ? (
        <div
          role="alert"
          className="space-y-4 rounded-xl border border-border p-6"
        >
          <p className="text-sm text-error">{t('membersLoadFailed')}</p>
          <Button
            variant="outline"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.members.length === 0 ? (
        <section className="flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-14 text-center">
          <Users
            className="mb-4 size-8 text-muted-foreground"
            aria-hidden="true"
          />
          <h2 className="font-medium">{t('noMembers')}</h2>
          <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
            {t('noMembersDescription')}
          </p>
        </section>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border text-muted-foreground">
              <tr>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('username')}
                </th>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('memberStatus')}
                </th>
                <th scope="col" className="px-5 py-3 font-medium">
                  {t('memberCreatedAt')}
                </th>
                <th scope="col" className="px-5 py-3 text-right font-medium">
                  {t('actions')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {query.data.members.map((member) => (
                <tr key={member.id}>
                  <td className="max-w-64 break-words px-5 py-4 font-medium">
                    {member.username}
                  </td>
                  <td className="px-5 py-4">
                    <Status kind={member.enabled ? 'success' : 'neutral'}>
                      {t(member.enabled ? 'active' : 'disabled')}
                    </Status>
                  </td>
                  <td className="whitespace-nowrap px-5 py-4 text-muted-foreground">
                    <time
                      dateTime={new Date(
                        member.created_at * 1000,
                      ).toISOString()}
                    >
                      {new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
                        dateStyle: 'medium',
                      }).format(member.created_at * 1000)}
                    </time>
                  </td>
                  <td className="px-5 py-4 text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={update.isPending}
                      aria-label={t(
                        member.enabled
                          ? 'disableMemberNamed'
                          : 'enableMemberNamed',
                        { username: member.username },
                      )}
                      onClick={() =>
                        update.mutate({
                          id: member.id,
                          enabled: !member.enabled,
                        })
                      }
                    >
                      {update.isPending &&
                        update.variables.id === member.id && (
                          <LoaderCircle
                            className="motion-safe:animate-spin"
                            aria-hidden="true"
                          />
                        )}
                      {t(member.enabled ? 'disableMember' : 'enableMember')}
                    </Button>
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
            disabled={
              cursors.length === 1 || query.isFetching || update.isPending
            }
            onClick={() => setCursors((current) => current.slice(0, -1))}
          >
            {t('previousPage')}
          </Button>
          <Button
            variant="outline"
            disabled={
              !query.data.next_cursor || query.isFetching || update.isPending
            }
            onClick={() =>
              setCursors((current) => [...current, query.data.next_cursor])
            }
          >
            {t('nextPage')}
          </Button>
        </div>
      )}
      {creating && (
        <CreateMember
          onClose={() => setCreating(false)}
          onCreated={(member) => {
            setCreatedName(member.username)
            setCreating(false)
            setCursors([0])
            return client.invalidateQueries({ queryKey: ['members'] })
          }}
        />
      )}
    </div>
  )
}
