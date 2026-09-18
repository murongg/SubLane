import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'

export function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="py-12">
      <h1 className="page-title">{t('notFoundTitle')}</h1>
      <p className="page-description">{t('notFoundDescription')}</p>
      <Button asChild className="mt-6">
        <Link to="/">{t('backToOverview')}</Link>
      </Button>
    </div>
  )
}
