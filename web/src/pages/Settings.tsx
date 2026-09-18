import { useTranslation } from 'react-i18next'
import { LanguageSelect, ThemeSelect } from '@/components/Preferences'

export function Settings() {
  const { t } = useTranslation()
  return (
    <div className="max-w-3xl">
      <h1 className="page-title">{t('preferences')}</h1>
      <p className="page-description">{t('preferencesDescription')}</p>
      <div className="mt-8 divide-y divide-border border-y border-border">
        <section className="flex flex-wrap items-center justify-between gap-4 py-6">
          <div>
            <h2 className="font-medium">{t('appearance')}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {t('appearanceDescription')}
            </p>
          </div>
          <ThemeSelect />
        </section>
        <section className="flex flex-wrap items-center justify-between gap-4 py-6">
          <div>
            <h2 className="font-medium">{t('language')}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {t('languageDescription')}
            </p>
          </div>
          <LanguageSelect />
        </section>
      </div>
    </div>
  )
}
