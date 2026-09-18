import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowUpRight,
  CircleAlert,
  LoaderCircle,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getSystem } from '@/lib/api'
import { Button } from '@/components/ui/Button'
import { Status } from '@/components/Status'

export function Overview() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['system'],
    queryFn: ({ signal }) => getSystem(signal),
  })
  return (
    <div className="space-y-8">
      <div>
        <h1 className="page-title">{t('overviewTitle')}</h1>
        <p className="page-description">{t('overviewDescription')}</p>
      </div>
      {query.isPending ? (
        <div
          role="status"
          className="flex min-h-48 items-center justify-center gap-3 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loading')}
        </div>
      ) : query.isError ? (
        <div
          role="alert"
          className="rounded-lg border border-border bg-card p-6"
        >
          <div className="flex items-center gap-2 font-medium text-error">
            <CircleAlert className="size-4" aria-hidden="true" />
            {t('connectionError')}
          </div>
          <p className="my-3 text-sm text-muted-foreground">
            {t(
              query.error.message === 'invalid_response'
                ? 'invalidResponse'
                : 'unavailable',
            )}
          </p>
          <Button
            variant="outline"
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : (
        <section
          aria-labelledby="service-title"
          className="rounded-xl border border-border bg-card"
        >
          <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border px-6 py-5">
            <div>
              <h2 id="service-title" className="font-medium">
                {t('service')}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('serviceDescription')}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <Status kind="success">{t('running')}</Status>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => void query.refetch()}
                disabled={query.isFetching}
                aria-label={t(query.isFetching ? 'refreshing' : 'refresh')}
              >
                <RefreshCw
                  className={query.isFetching ? 'motion-safe:animate-spin' : ''}
                  aria-hidden="true"
                />
              </Button>
            </div>
          </div>
          <dl className="grid gap-6 px-6 py-6 sm:grid-cols-3">
            <div>
              <dt className="text-sm text-muted-foreground">{t('version')}</dt>
              <dd className="mt-2 text-sm font-medium">{query.data.version}</dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">{t('uptime')}</dt>
              <dd className="mt-2 text-sm font-medium tabular-nums">
                {t('uptimeValue', { seconds: query.data.uptime_seconds })}
              </dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">{t('storage')}</dt>
              <dd className="mt-2 text-sm font-medium">
                SQLite{' '}
                <span className="text-muted-foreground">/ {t('ready')}</span>
              </dd>
            </div>
          </dl>
        </section>
      )}
      <section className="flex flex-col justify-between gap-5 border-y border-border py-6 sm:flex-row sm:items-center">
        <div className="max-w-xl">
          <div className="mb-2 flex flex-wrap items-center gap-3">
            <h2 className="font-medium">{t('codexTitle')}</h2>
            <Status kind="neutral">{t('notConfigured')}</Status>
          </div>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('codexDescription')}
          </p>
        </div>
        <Button asChild variant="outline">
          <Link to="/accounts">
            {t('viewAccounts')}
            <ArrowUpRight aria-hidden="true" />
          </Link>
        </Button>
      </section>
    </div>
  )
}
