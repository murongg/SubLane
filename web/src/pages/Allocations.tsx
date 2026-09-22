import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import {
  schemesOptions,
  availableAllocationPools,
  teamsOptions,
  saveScheme,
  setSchemeEnabled,
  modeLabels,
  allocationErrorKey,
  type Scheme,
} from '@/lib/allocations'
import { groupOptions } from '@/lib/groups'
import { SchemeForm } from '@/components/SchemeForm'
import { AllocationReport } from '@/components/AllocationReport'
import { Button } from '@/components/ui/Button'
import { Status } from '@/components/Status'

export function Allocations() {
  const { t, i18n } = useTranslation()
  const query = useQuery(schemesOptions)
  const teams = useQuery(teamsOptions)
  const pools = useQuery(groupOptions)
  const client = useQueryClient()
  const [editing, setEditing] = useState<Scheme | null | undefined>()
  const [viewing, setViewing] = useState<number | null>(null)
  const saved = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['allocations'] }),
      client.invalidateQueries({ queryKey: ['allocation'] }),
      client.invalidateQueries({ queryKey: ['own-allocations'] }),
      client.invalidateQueries({ queryKey: ['available-groups'] }),
      client.invalidateQueries({ queryKey: ['keys'] }),
    ])
    setEditing(undefined)
  }
  const mutation = useMutation({ mutationFn: saveScheme, onSuccess: saved })
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      setSchemeEnabled(id, enabled),
    onSuccess: saved,
  })
  const freePools = availableAllocationPools(
    pools.data?.groups ?? [],
    query.data?.schemes ?? [],
  )
  const activeTeams =
    teams.data?.teams.filter(
      (team) => team.enabled && team.members.length > 0,
    ) ?? []
  const loading = query.isPending || teams.isPending || pools.isPending
  const failed = query.isError || teams.isError || pools.isError
  const date = (n: number) =>
    new Date(n * 1000).toLocaleString(i18n.resolvedLanguage ?? 'en')
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="page-title">{t('allocationSchemes')}</h1>
          <p className="page-description">{t('allocationsDescription')}</p>
        </div>
        {editing === undefined && viewing === null && (
          <Button
            disabled={
              loading || failed || !activeTeams.length || !freePools.length
            }
            onClick={() => {
              mutation.reset()
              setEditing(null)
            }}
          >
            <Plus aria-hidden="true" />
            {t('allocationCreate')}
          </Button>
        )}
      </div>
      {!loading &&
        !failed &&
        editing === undefined &&
        viewing === null &&
        (!freePools.length || !activeTeams.length) && (
          <p className="text-sm leading-6 text-muted-foreground">
            {t('allocationPrerequisite')}{' '}
            <Link className="underline underline-offset-4" to="/groups">
              {t('accountGroups')}
            </Link>{' '}
            ·{' '}
            <Link className="underline underline-offset-4" to="/admin/teams">
              {t('personnelTeams')}
            </Link>
          </p>
        )}
      {toggle.isError && (
        <p role="alert" className="text-sm text-error">
          {t(allocationErrorKey(toggle.error))}
        </p>
      )}
      {loading ? (
        <p role="status">{t('allocationLoading')}</p>
      ) : failed ? (
        <div role="alert" className="space-y-3">
          <p>{t('allocationFailed')}</p>
          <Button
            variant="outline"
            onClick={() =>
              Promise.all([query.refetch(), teams.refetch(), pools.refetch()])
            }
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : editing !== undefined ? (
        <section className="max-w-3xl space-y-5">
          <h2 className="font-medium">
            {t(editing ? 'allocationEdit' : 'allocationCreate')}
          </h2>
          <SchemeForm
            key={editing?.id ?? 'new'}
            teams={editing ? teams.data.teams : activeTeams}
            groups={editing ? pools.data.groups : freePools}
            scheme={editing ?? undefined}
            onSubmit={(input) => mutation.mutate({ ...input, id: editing?.id })}
            onCancel={() => setEditing(undefined)}
            pending={mutation.isPending}
          />
          {mutation.isError && (
            <p role="alert" className="text-sm text-error">
              {t(allocationErrorKey(mutation.error))}
            </p>
          )}
        </section>
      ) : viewing !== null ? (
        <>
          <Button variant="outline" onClick={() => setViewing(null)}>
            {t('allocationBack')}
          </Button>
          <AllocationReport id={viewing} />
        </>
      ) : query.data.schemes.length === 0 ? (
        <div className="space-y-4 py-8">
          <p className="text-sm text-muted-foreground">
            {t('allocationsEmpty')}
          </p>
          <Button asChild variant="outline">
            <Link to="/admin/teams">{t('personnelTeams')}</Link>
          </Button>
        </div>
      ) : (
        <div className="divide-y divide-border rounded-xl border border-border">
          {query.data.schemes.map((s) => (
            <section
              key={s.id}
              className="flex flex-wrap items-center justify-between gap-4 p-5"
            >
              <div className="min-w-0 space-y-2">
                <div className="flex flex-wrap items-center gap-3">
                  <h2 className="break-words font-medium">{s.name}</h2>
                  <Status
                    kind={
                      s.enabled && s.effective_at <= query.dataUpdatedAt / 1000
                        ? 'success'
                        : 'neutral'
                    }
                  >
                    {t(
                      !s.enabled
                        ? 'disabled'
                        : s.effective_at > query.dataUpdatedAt / 1000
                          ? 'allocationWaiting'
                          : 'active',
                    )}
                  </Status>
                  <span className="text-sm text-muted-foreground">
                    {t(modeLabels[s.config.mode])}
                  </span>
                </div>
                <p className="break-words text-sm text-muted-foreground">
                  {s.team_name} · {s.group_name} ·{' '}
                  {t(
                    s.config.period === 'upstream'
                      ? 'allocationUpstreamReset'
                      : s.config.period === 'day'
                        ? 'budgetDaily'
                        : 'budgetMonthly',
                  )}
                </p>
                {s.next && (
                  <p className="text-sm text-muted-foreground">
                    {t('allocationScheduled', {
                      mode: t(modeLabels[s.next.config.mode]),
                      date: date(s.next.effective_at),
                    })}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={toggle.isPending}
                  onClick={() =>
                    toggle.mutate({ id: s.id, enabled: !s.enabled })
                  }
                >
                  {t(s.enabled ? 'allocationPause' : 'allocationResume')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setViewing(s.id)}
                >
                  {t('allocationView')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={t('allocationEditNamed', { name: s.name })}
                  onClick={() => {
                    mutation.reset()
                    setEditing(s)
                  }}
                >
                  {t('allocationEdit')}
                </Button>
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
