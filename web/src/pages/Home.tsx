import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { authOptions } from '@/lib/auth'
import { Status } from '@/components/Status'
import { Button } from '@/components/ui/Button'
import { Overview } from './Overview'

export function Home() {
  const { data } = useQuery(authOptions())
  const { t } = useTranslation()
  const user = data?.user
  if (!user) return null
  if (user.role === 'admin') return <Overview />
  return (
    <div className="max-w-3xl space-y-8">
      <div>
        <h1 className="page-title">{t('memberOverviewTitle')}</h1>
        <p className="page-description">{t('memberOverviewDescription')}</p>
      </div>
      <section className="rounded-xl border border-border bg-card p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-semibold">{t('yourAccount')}</h2>
          <Status kind="success">{t('active')}</Status>
        </div>
        <dl className="mt-6 grid gap-5 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">{t('username')}</dt>
            <dd className="mt-2 font-medium">{user.username}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('role')}</dt>
            <dd className="mt-2 font-medium">{t('member')}</dd>
          </div>
        </dl>
      </section>
      <section>
        <h2 className="font-semibold">{t('clientAccess')}</h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t('memberAccessDescription')}
        </p>
        <Button asChild variant="outline" className="mt-4">
          <Link to="/keys">{t('manageKeys')}</Link>
        </Button>
      </section>
    </div>
  )
}
