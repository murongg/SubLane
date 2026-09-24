import { useTranslation } from 'react-i18next'
import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { authOptions } from '@/lib/auth'
import { PasswordDialog } from '@/components/PasswordDialog'
import { Button } from '@/components/ui/Button'
import { LanguageSelect, ThemeSelect } from '@/components/Preferences'

export function Settings() {
  const { t } = useTranslation()
  const { data } = useQuery(authOptions())
  const [changing, setChanging] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  return (
    <div className="max-w-3xl">
      <h1 className="page-title">{t('preferences')}</h1>
      <div className="mt-8 divide-y divide-border border-y border-border">
        <section className="flex flex-wrap items-center justify-between gap-4 py-6">
          <div>
            <h2 className="font-medium">{t('appearance')}</h2>
          </div>
          <ThemeSelect />
        </section>
        <section className="flex flex-wrap items-center justify-between gap-4 py-6">
          <div>
            <h2 className="font-medium">{t('language')}</h2>
          </div>
          <LanguageSelect />
        </section>
        <section className="flex flex-wrap items-center justify-between gap-4 py-6">
          <div>
            <h2 className="font-medium">{t('password')}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {t('passwordSessionNotice')}
            </p>
          </div>
          <Button
            ref={trigger}
            variant="outline"
            onClick={() => setChanging(true)}
          >
            {t('changePassword')}
          </Button>
        </section>
      </div>
      {changing && data?.user && (
        <PasswordDialog
          key={data.user.id}
          onClose={() => setChanging(false)}
          returnFocus={() => trigger.current?.focus()}
        />
      )}
    </div>
  )
}
