import { Link } from '@tanstack/react-router'
import { ShieldX } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'

export function Forbidden() {
  const { t } = useTranslation()
  return (
    <section className="mx-auto flex max-w-lg flex-col items-center py-16 text-center">
      <ShieldX
        className="mb-5 size-8 text-muted-foreground"
        aria-hidden="true"
      />
      <h1 className="page-title">{t('accessDenied')}</h1>
      <p className="page-description">{t('adminAccessRequired')}</p>
      <Button asChild className="mt-6">
        <Link to="/">{t('backToOverview')}</Link>
      </Button>
    </section>
  )
}
