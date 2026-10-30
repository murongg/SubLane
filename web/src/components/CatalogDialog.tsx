import { useEffect, useId, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { List, LoaderCircle, Play, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  catalogOptions,
  checkModel,
  refreshCatalog,
  type CatalogTarget,
  type ModelCheck,
  type ModelCapabilities,
} from '@/lib/catalog'
import { authKey, type AuthState } from '@/lib/auth'
import { availabilityReasons } from '@/lib/availability'
import { accountErrorKey } from '@/lib/accounts'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { ModelAvailability } from './ModelAvailability'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'

import { useTimeZone } from '@/lib/timezone'
export function CatalogDialog({
  target,
  name,
  disabled = false,
}: {
  target: CatalogTarget
  name: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled}
          aria-label={t('catalogFor', { name })}
        >
          <List aria-hidden="true" />
          {t('catalogModels')}
        </Button>
      </DialogTrigger>
      {open && <CatalogContent target={target} name={name} />}
    </Dialog>
  )
}
function CatalogContent({
  target,
  name,
}: {
  target: CatalogTarget
  name: string
}) {
  const { t, i18n } = useTranslation()
  const individual = target.kind === 'account' || target.kind === 'channel'
  const timeZone = useTimeZone()
  const client = useQueryClient()
  const options = catalogOptions(client, target)
  const query = useQuery(options)
  const [filter, setFilter] = useState('')
  const [inspectedModel, setInspectedModel] = useState('')
  const [now, setNow] = useState(Date.now)
  const [ownerID] = useState(
    () => client.getQueryData<AuthState>(authKey)?.user?.id,
  )
  const [checks, setChecks] = useState<Record<string, ModelCheck>>({})
  const check = useAdminMutation({
    userID: ownerID ?? 0,
    mutationFn: (model: string, signal) =>
      checkModel(
        {
          kind: target.kind === 'channel' ? 'channel' : 'account',
          id: String(target.id),
        },
        model,
        signal,
      ),
    onSuccess: (result) =>
      setChecks((previous) => ({ ...previous, [result.model]: result })),
  })
  const refresh = useAdminMutation({
    userID: ownerID ?? 0,
    mutationFn: (_input: void, signal) =>
      refreshCatalog(
        {
          kind: target.kind === 'channel' ? 'channel' : 'account',
          id: String(target.id),
        },
        signal,
      ),
    onSuccess: (data) => client.setQueryData(options.queryKey, data),
  })
  const data = query.data
  const labelID = useId()
  const cooldown = Math.max(
    0,
    (data?.retry_after_seconds ?? 0) -
      Math.floor(Math.max(0, now - query.dataUpdatedAt) / 1000),
  )
  useEffect(() => {
    if (!cooldown) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])
  const pending =
    query.isFetching ||
    refresh.isPending ||
    check.isPending ||
    Boolean(data?.refreshing)
  const failed =
    query.isError || refresh.isError || Boolean(data?.refresh_failed)
  const catalogModels = [
    ...(data?.models ?? []),
    ...(data?.hidden_models ?? []),
  ]
  const models = catalogModels.filter((model) =>
    model.toLowerCase().includes(filter.toLowerCase()),
  )
  return (
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>{t('catalogFor', { name })}</DialogTitle>
        <DialogDescription>
          {t(
            individual
              ? target.kind === 'channel'
                ? 'catalogChannelDescription'
                : 'catalogAccountDescription'
              : 'catalogGroupDescription',
          )}
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
        <span>
          {data?.known
            ? t('catalogCount', { count: catalogModels.length })
            : t('catalogUnknown')}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={pending || (individual && cooldown > 0)}
          onClick={() => (individual ? refresh.mutate() : query.refetch())}
        >
          <RefreshCw
            aria-hidden="true"
            className={pending ? 'motion-safe:animate-spin' : undefined}
          />
          {t(individual ? 'catalogRefresh' : 'catalogReload')}
        </Button>
      </div>
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-4 motion-safe:animate-spin"
          />
          {t('catalogLoading')}
        </p>
      ) : (
        <>
          {failed && (
            <p role="status" className="text-sm text-warning">
              {t(data?.known ? 'catalogRefreshFailed' : 'catalogLoadFailed')}
            </p>
          )}
          {data?.refreshing && (
            <p role="status" className="text-sm text-muted-foreground">
              {t('catalogRefreshing')}
            </p>
          )}
          {data?.partial && (
            <p role="status" className="text-sm text-warning">
              {t('catalogPartial')}
            </p>
          )}
          {data?.known && data.stale && (!data.usable || !failed) && (
            <p className="text-sm text-muted-foreground">
              {t(data.usable ? 'catalogStale' : 'catalogExpired')}
            </p>
          )}
          {data && !data.known && (
            <p className="text-sm text-muted-foreground">
              {t('catalogUnknownHint')}
            </p>
          )}
          {data?.known && catalogModels.length === 0 && (
            <p className="py-4 text-sm text-muted-foreground">
              {t(individual ? 'catalogEmptyAccount' : 'catalogEmptyGroup')}
            </p>
          )}
          {Boolean(catalogModels.length) && (
            <>
              {individual && (
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('modelTestHint')}
                </p>
              )}
              <p className="text-xs leading-5 text-muted-foreground">
                {t('catalogCapabilitiesHint')}
                {!individual && <> {t('catalogCapabilitiesMerge')}</>}
              </p>
              <Input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder={t('catalogSearch')}
                aria-label={t('catalogSearch')}
              />
              <ul
                className="max-h-72 overflow-y-auto divide-y divide-border"
                aria-label={t('catalogAvailableModels')}
              >
                {models.slice(0, 200).map((model) => {
                  const checking = check.isPending && check.variables === model
                  const error = check.isError && check.variables === model
                  const result = checks[model]
                  const hidden = data?.hidden_models.includes(model)
                  const hiddenID = `${labelID}-${model}`
                  const label = (
                    <span className="flex min-w-0 flex-wrap items-center gap-2">
                      <span className="min-w-0 break-all">{model}</span>
                      {hidden && (
                        <span
                          id={hiddenID}
                          className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 font-sans text-xs text-muted-foreground"
                        >
                          {t('catalogHiddenModel')}
                        </span>
                      )}
                    </span>
                  )
                  return (
                    <li
                      key={model}
                      className="break-all py-2.5 font-mono text-xs"
                    >
                      {individual ? (
                        <>
                          <div className="flex items-center justify-between gap-3">
                            {label}
                            <Button
                              variant="ghost"
                              size="sm"
                              className="shrink-0 font-sans"
                              disabled={pending || !data?.usable}
                              aria-label={t(
                                checking ? 'modelTestingFor' : 'modelTestFor',
                                { model },
                              )}
                              onClick={() => check.mutate(model)}
                            >
                              {checking ? (
                                <LoaderCircle
                                  aria-hidden="true"
                                  className="motion-safe:animate-spin"
                                />
                              ) : (
                                <Play aria-hidden="true" />
                              )}
                              {t(checking ? 'modelTesting' : 'modelTest')}
                            </Button>
                          </div>
                          {!checking && (error || result) && (
                            <p
                              role="status"
                              className={`mt-1 font-sans ${!error && result?.available ? 'text-success' : 'text-warning'}`}
                            >
                              {error
                                ? t(accountErrorKey(check.error))
                                : t('modelTestResult', {
                                    result: t(
                                      result.available
                                        ? 'modelTestSuccess'
                                        : (availabilityReasons[
                                            result.error_code
                                          ] ?? 'reasonUnknown'),
                                    ),
                                    duration: result.duration_ms,
                                  })}
                            </p>
                          )}
                        </>
                      ) : (
                        <button
                          type="button"
                          className="w-full break-all rounded-sm text-left hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
                          aria-label={t('availabilityInspect', { model })}
                          aria-describedby={hidden ? hiddenID : undefined}
                          onClick={() => setInspectedModel(model)}
                        >
                          {label}
                        </button>
                      )}
                      <CapabilityDetails
                        capabilities={data?.capabilities[model]}
                      />
                    </li>
                  )
                })}
              </ul>
              {!models.length && (
                <p className="text-sm text-muted-foreground">
                  {t('catalogNoMatch')}
                </p>
              )}
              {models.length > 200 && (
                <p className="text-xs text-muted-foreground">
                  {t('catalogNarrowSearch')}
                </p>
              )}
            </>
          )}
          {Boolean(data?.updated_at) && (
            <p className="text-xs text-muted-foreground">
              {t('catalogUpdated', {
                time: new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
                  timeZone,
                  dateStyle: 'short',
                  timeStyle: 'short',
                }).format(data!.updated_at * 1000),
              })}
            </p>
          )}
        </>
      )}
      {(target.kind === 'group' || target.kind === 'key') && (
        <ModelAvailability
          key={inspectedModel}
          target={{ kind: target.kind, id: Number(target.id) }}
          model={inspectedModel}
          onInspect={setInspectedModel}
        />
      )}
    </DialogContent>
  )
}

function CapabilityDetails({
  capabilities,
}: {
  capabilities?: ModelCapabilities
}) {
  const { t, i18n } = useTranslation()
  const unknown = t('catalogUnknown')
  const tokens = (value?: number) =>
    value === undefined
      ? unknown
      : new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en').format(value)
  const support = (value?: boolean) =>
    value === undefined
      ? unknown
      : t(value ? 'catalogSupported' : 'catalogUnsupported')
  const details = [
    [t('catalogContextWindow'), tokens(capabilities?.context_window)],
    [t('catalogMaxOutput'), tokens(capabilities?.max_output_tokens)],
    [
      t('catalogReasoningLevels'),
      capabilities?.supported_reasoning_levels?.length
        ? capabilities.supported_reasoning_levels.join(', ')
        : unknown,
    ],
    [t('catalogImages'), support(capabilities?.supports_images)],
    [t('catalogTools'), support(capabilities?.supports_tools)],
  ]
  return (
    <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 font-sans text-xs sm:grid-cols-3">
      {details.map(([label, value]) => (
        <div key={label} className="min-w-0">
          <dt className="text-muted-foreground">{label}</dt>
          <dd className="break-words text-foreground">{value}</dd>
        </div>
      ))}
    </dl>
  )
}
