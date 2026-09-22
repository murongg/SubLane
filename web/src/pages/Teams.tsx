import { useState } from 'react'
import {
  useQuery,
  useMutation,
  useQueryClient,
  useInfiniteQuery,
} from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import {
  teamsOptions,
  schemesOptions,
  saveScheme,
  availableAllocationPools,
  modeLabels,
  saveTeam,
  allocationErrorKey,
  type Team,
  type Scheme,
} from '@/lib/allocations'
import { getMembers } from '@/lib/members'
import { groupOptions } from '@/lib/groups'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Status } from '@/components/Status'
import { SchemeForm } from '@/components/SchemeForm'
import { AllocationReport } from '@/components/AllocationReport'

export function Teams() {
  const { t } = useTranslation()
  const query = useQuery(teamsOptions)
  const schemes = useQuery(schemesOptions)
  const pools = useQuery(groupOptions)
  const client = useQueryClient()
  const [editing, setEditing] = useState<Team | null | undefined>()
  const [resourceEditing, setResourceEditing] = useState<
    Scheme | null | undefined
  >()
  const mutation = useMutation({
    mutationFn: saveTeam,
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['teams'] }),
        client.invalidateQueries({ queryKey: ['own-allocations'] }),
        client.invalidateQueries({ queryKey: ['available-groups'] }),
        client.invalidateQueries({ queryKey: ['keys'] }),
      ])
      setEditing(undefined)
      setResourceEditing(undefined)
    },
  })
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="page-title">{t('personnelTeams')}</h1>
          <p className="page-description">{t('teamsDescription')}</p>
        </div>
        {editing === undefined && (
          <Button
            onClick={() => {
              mutation.reset()
              setEditing(null)
            }}
          >
            <Plus aria-hidden="true" />
            {t('teamCreate')}
          </Button>
        )}
      </div>
      {editing !== undefined ? (
        <section className="max-w-3xl space-y-6">
          <div className="space-y-1">
            <h2 className="font-medium">
              {t(editing ? 'teamEdit' : 'teamCreate')}
            </h2>
            <p className="text-sm leading-6 text-muted-foreground">
              {t('teamEditDescription')}
            </p>
          </div>
          <TeamForm
            key={editing?.id ?? 'new'}
            team={editing ?? undefined}
            pending={mutation.isPending}
            onCancel={() => setEditing(undefined)}
            onSave={(input) => mutation.mutate(input)}
          />
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(allocationErrorKey(mutation.error))}
            </p>
          )}
          {editing && (
            <TeamResources
              team={editing}
              schemes={schemes.data?.schemes ?? []}
              groups={pools.data?.groups ?? []}
              poolsLoading={pools.isPending}
              editing={resourceEditing}
              onEdit={setResourceEditing}
              onSaved={async () => {
                await Promise.all([
                  client.invalidateQueries({ queryKey: ['allocations'] }),
                  client.invalidateQueries({ queryKey: ['available-groups'] }),
                  client.invalidateQueries({ queryKey: ['keys'] }),
                ])
                setResourceEditing(undefined)
              }}
              onCancel={() => setResourceEditing(undefined)}
            />
          )}
        </section>
      ) : query.isPending ? (
        <p role="status">{t('allocationLoading')}</p>
      ) : query.isError ? (
        <div role="alert" className="space-y-3">
          <p>{t('allocationFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.teams.length === 0 ? (
        <p className="py-8 text-sm text-muted-foreground">{t('teamsEmpty')}</p>
      ) : (
        <div className="divide-y divide-border rounded-xl border border-border">
          {query.data.teams.map((team) => (
            <section
              key={team.id}
              className="flex flex-wrap items-center justify-between gap-4 p-5"
            >
              <div className="min-w-0 space-y-2">
                <div className="flex flex-wrap items-center gap-3">
                  <h2 className="break-words font-medium">{team.name}</h2>
                  <Status kind={team.enabled ? 'success' : 'neutral'}>
                    {t(team.enabled ? 'active' : 'disabled')}
                  </Status>
                </div>
                <p className="text-sm text-muted-foreground">
                  {t('teamMemberCount', { count: team.member_ids.length })}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t('teamResourceCount', {
                    count:
                      schemes.data?.schemes.filter(
                        (scheme) => scheme.team_id === team.id,
                      ).length ?? 0,
                  })}
                </p>
                <p className="max-w-2xl break-words text-sm text-muted-foreground">
                  {team.members.map((m) => m.username).join(', ')}
                </p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  mutation.reset()
                  setResourceEditing(undefined)
                  setEditing(team)
                }}
                aria-label={t('teamEditNamed', { name: team.name })}
              >
                {t('teamEdit')}
              </Button>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}

function TeamResources({
  team,
  schemes,
  groups,
  poolsLoading,
  editing,
  onEdit,
  onSaved,
  onCancel,
}: {
  team: Team
  schemes: Scheme[]
  groups: {
    id: number
    name: string
    enabled: boolean
    account_count: number
  }[]
  poolsLoading: boolean
  editing: Scheme | null | undefined
  onEdit: (scheme: Scheme | null) => void
  onSaved: () => Promise<void>
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [viewing, setViewing] = useState<number | null>(null)
  const mutation = useMutation({
    mutationFn: saveScheme,
    onSuccess: onSaved,
  })
  const teamSchemes = schemes.filter((scheme) => scheme.team_id === team.id)
  const available = availableAllocationPools(
    groups,
    schemes.filter((scheme) => scheme.id !== editing?.id),
  )
  const resourceGroups = editing
    ? groups.filter(
        (group) =>
          group.id === editing.group_id ||
          available.some((v) => v.id === group.id),
      )
    : available
  return (
    <section className="space-y-5 border-t border-border pt-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-medium">{t('teamResources')}</h3>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            {t('teamResourcesHint')}
          </p>
          <p className="mt-2 text-sm font-medium">{t('teamResourceFormula')}</p>
        </div>
        {editing === undefined && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!available.length}
            onClick={() => {
              mutation.reset()
              onEdit(null)
            }}
          >
            <Plus aria-hidden="true" />
            {t('teamResourceAdd')}
          </Button>
        )}
      </div>
      {viewing !== null ? (
        <div className="space-y-4">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setViewing(null)}
          >
            {t('allocationBack')}
          </Button>
          <AllocationReport id={viewing} />
        </div>
      ) : poolsLoading ? (
        <p role="status">{t('allocationLoading')}</p>
      ) : editing !== undefined ? (
        <div className="space-y-4">
          <h4 className="font-medium">
            {t(editing ? 'teamResourceEdit' : 'teamResourceAdd')}
          </h4>
          {!resourceGroups.length ? (
            <p className="text-sm text-muted-foreground">
              {t('teamResourceNoPools')}
            </p>
          ) : (
            <SchemeForm
              key={editing?.id ?? 'new-resource'}
              teams={[team]}
              groups={resourceGroups}
              scheme={editing ?? undefined}
              fixedTeam={team}
              resourceMode
              onSubmit={(input) =>
                mutation.mutate({ ...input, id: editing?.id })
              }
              onCancel={onCancel}
              pending={mutation.isPending}
            />
          )}
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(allocationErrorKey(mutation.error))}
            </p>
          )}
        </div>
      ) : teamSchemes.length ? (
        <div className="divide-y divide-border rounded-lg border border-border">
          {teamSchemes.map((scheme) => (
            <div
              key={scheme.id}
              className="flex flex-wrap items-center justify-between gap-3 p-4"
            >
              <div className="min-w-0 space-y-1">
                <p className="break-words text-sm font-medium">
                  {t('teamResourcePoolLabel')}: {scheme.group_name}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t('teamResourceLimitLabel', {
                    mode: t(modeLabels[scheme.config.mode]),
                  })}{' '}
                  ·{' '}
                  {t(
                    scheme.config.period === 'upstream'
                      ? 'allocationUpstreamReset'
                      : scheme.config.period === 'day'
                        ? 'budgetDaily'
                        : 'budgetMonthly',
                  )}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Status kind={scheme.enabled ? 'success' : 'neutral'}>
                  {t(scheme.enabled ? 'active' : 'disabled')}
                </Status>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setViewing(scheme.id)}
                >
                  {t('allocationView')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    mutation.reset()
                    onEdit(scheme)
                  }}
                >
                  {t('teamResourceEdit')}
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {available.length
            ? t('teamResourcesEmpty')
            : t('teamResourceNoPools')}
        </p>
      )}
    </section>
  )
}
function TeamForm({
  team,
  pending,
  onSave,
  onCancel,
}: {
  team?: Team
  pending: boolean
  onSave: (input: {
    id?: number
    name: string
    enabled: boolean
    member_ids: number[]
    group_ids: number[]
  }) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(team?.name ?? '')
  const [enabled, setEnabled] = useState(team?.enabled ?? true)
  const [ids, setIDs] = useState(team?.member_ids ?? [])
  const [groupIDs, setGroupIDs] = useState(team?.group_ids ?? [])
  const [invalid, setInvalid] = useState(false)
  const members = useInfiniteQuery({
    queryKey: ['team-member-directory'],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => getMembers(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor || undefined,
  })
  const groups = useQuery(groupOptions)
  const all = new Map(team?.members.map((m) => [m.id, m]) ?? [])
  for (const page of members.data?.pages ?? [])
    for (const m of page.members) all.set(m.id, m)
  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        if (pending) return
        if (!name.trim() || ids.length > 100) {
          setInvalid(true)
          return
        }
        onSave({
          id: team?.id,
          name: name.trim(),
          enabled,
          member_ids: ids,
          group_ids: groupIDs,
        })
      }}
    >
      <fieldset disabled={pending} className="space-y-5">
        <div className="space-y-1">
          <h3 className="font-medium">{t('teamPeopleTitle')}</h3>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('teamPeopleHint')}
          </p>
        </div>
        <div className="space-y-2">
          <label htmlFor="team-name" className="text-sm font-medium">
            {t('teamName')}
          </label>
          <Input
            id="team-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={64}
          />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          {t('teamEnabled')}
        </label>
        <fieldset className="space-y-3">
          <legend className="text-sm font-medium">
            {t('allocationMembers')}
          </legend>
          {members.isPending && <p role="status">{t('allocationLoading')}</p>}
          {members.isError && (
            <div role="alert">
              <p>{t('allocationFailed')}</p>
              <Button
                type="button"
                variant="outline"
                onClick={() => members.refetch()}
              >
                {t('reconnect')}
              </Button>
            </div>
          )}
          <div className="grid max-h-80 gap-3 overflow-y-auto sm:grid-cols-2">
            {[...all.values()].map((m) => (
              <label key={m.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={ids.includes(m.id)}
                  className="size-4 accent-primary"
                  onChange={(e) =>
                    setIDs((all) =>
                      e.target.checked
                        ? [...all, m.id]
                        : all.filter((id) => id !== m.id),
                    )
                  }
                />
                <span className="break-words">
                  {m.username}
                  {!m.enabled && ` · ${t('disabled')}`}
                </span>
              </label>
            ))}
          </div>
          {members.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              onClick={() => members.fetchNextPage()}
              disabled={members.isFetchingNextPage}
            >
              {t('allocationMoreMembers')}
            </Button>
          )}
        </fieldset>
        <fieldset className="space-y-3">
          <legend className="text-sm font-medium">
            {t('teamAllowedGroups')}
          </legend>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('teamAllowedGroupsHint')}
          </p>
          {groups.isPending && <p role="status">{t('allocationLoading')}</p>}
          {groups.isError && <p role="alert">{t('groupsLoadFailed')}</p>}
          <div className="grid max-h-64 gap-3 overflow-y-auto rounded-lg border border-border p-3 sm:grid-cols-2">
            {groups.data?.groups.map((group) => (
              <label key={group.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={groupIDs.includes(group.id)}
                  className="size-4 accent-primary"
                  onChange={(e) =>
                    setGroupIDs((all) =>
                      e.target.checked
                        ? [...all, group.id]
                        : all.filter((id) => id !== group.id),
                    )
                  }
                />
                <span className="break-words">
                  {group.name}
                  {!group.enabled && ` · ${t('disabled')}`}
                </span>
              </label>
            ))}
          </div>
        </fieldset>
        <p className="text-sm leading-6 text-muted-foreground">
          {t('teamMembershipHint')}
        </p>
      </fieldset>
      {invalid && (
        <p role="alert" className="text-sm text-error">
          {t('allocationInvalid')}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={onCancel}
          disabled={pending}
        >
          {t('cancel')}
        </Button>
        <Button
          disabled={
            pending ||
            members.isPending ||
            members.isError ||
            groups.isPending ||
            groups.isError
          }
        >
          {t('teamSave')}
        </Button>
      </div>
    </form>
  )
}
