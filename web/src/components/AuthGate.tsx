import { Navigate, Outlet, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authOptions } from '@/lib/auth'
import { canAccess } from '@/lib/access'
import { Forbidden } from '@/pages/Forbidden'
import { AuthShell } from './AuthShell'
import { Layout } from './Layout'
import { Button } from './ui/Button'

export function AuthGate() {
  const { t } = useTranslation()
  const query = useQuery(authOptions())
  const path = useRouterState({ select: (state) => state.location.pathname })
  const isSetup = path === '/setup' || path === '/setup/admin'
  if (query.isPending)
    return (
      <AuthShell>
        <div
          role="status"
          className="flex items-center gap-3 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('checkingSession')}
        </div>
      </AuthShell>
    )
  if (query.isError)
    return (
      <AuthShell>
        <div role="alert">
          <h1 className="text-xl font-semibold">{t('connectionError')}</h1>
          <p className="my-4 text-sm leading-6 text-muted-foreground">
            {t('unavailable')}
          </p>
          <Button onClick={() => query.refetch()} disabled={query.isFetching}>
            {t('reconnect')}
          </Button>
        </div>
      </AuthShell>
    )
  if (!query.data.initialized)
    return isSetup ? <Outlet /> : <Navigate to="/setup" replace />
  if (!query.data.user)
    return path === '/login' ? <Outlet /> : <Navigate to="/login" replace />
  if (isSetup || path === '/login') return <Navigate to="/" replace />
  if (!canAccess(path, query.data.user.role))
    return (
      <Layout>
        <Forbidden />
      </Layout>
    )
  return <Layout />
}
