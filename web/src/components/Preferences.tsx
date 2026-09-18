import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { setLanguage, type Language } from '@/lib/i18n'

const selectClass =
  'h-9 max-w-32 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring'

export function LanguageSelect() {
  const { t, i18n } = useTranslation()
  return (
    <select
      className={selectClass}
      aria-label={t('language')}
      value={i18n.resolvedLanguage}
      onChange={(e) => setLanguage(e.target.value as Language)}
    >
      <option value="en">English</option>
      <option value="zh">简体中文</option>
    </select>
  )
}

export function ThemeSelect() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  return (
    <select
      className={selectClass}
      aria-label={t('themeLabel')}
      value={theme ?? 'system'}
      onChange={(e) => setTheme(e.target.value)}
    >
      <option value="system">{t('system')}</option>
      <option value="light">{t('light')}</option>
      <option value="dark">{t('dark')}</option>
    </select>
  )
}
