import { Link } from '@tanstack/react-router'
import { Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'

export function Accounts() {
  const { t } = useTranslation()
  return (
    <div>
      <h1 className="page-title">{t('accountsTitle')}</h1>
      <p className="page-description">{t('accountsDescription')}</p>
      <section className="mt-8 flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-16 text-center">
        <Workflow
          className="mb-5 size-8 text-muted-foreground"
          aria-hidden="true"
        />
        <h2 className="font-medium">{t('accountEmptyTitle')}</h2>
        <p className="mb-6 mt-3 max-w-md text-sm leading-6 text-muted-foreground">
          {t('accountEmptyDescription')}
        </p>
        <Button asChild variant="outline">
          <Link to="/">{t('backToOverview')}</Link>
        </Button>
      </section>
    </div>
  )
}
