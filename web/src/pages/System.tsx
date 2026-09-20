import { useQuery } from '@tanstack/react-query'
import { Outlet } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CodexVersion } from '@/components/CodexVersion'
import { authOptions } from '@/lib/auth'

export function System() {
  const { t } = useTranslation()
  return (
    <div className="max-w-3xl">
      <h1 className="page-title">{t('systemSettings')}</h1>
      <p className="page-description">{t('systemSettingsDescription')}</p>
      <Outlet />
    </div>
  )
}

export function CodexSettings() {
  const { data } = useQuery(authOptions())
  return data?.user?.role === 'admin' ? (
    <CodexVersion key={data.user.id} userID={data.user.id} />
  ) : null
}
