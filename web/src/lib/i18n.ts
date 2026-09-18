import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { en } from '@/locales/en'
import { zh } from '@/locales/zh'

export type Language = 'en' | 'zh'

function savedLanguage(): Language {
  try {
    return localStorage.getItem('sublane-language') === 'zh' ? 'zh' : 'en'
  } catch {
    return 'en'
  }
}

i18n
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, zh: { translation: zh } },
    lng: savedLanguage(),
    fallbackLng: 'en',
    supportedLngs: ['en', 'zh'],
    interpolation: { escapeValue: false },
  })
  .catch((error: unknown) => {
    console.error('Could not initialize translations', error)
  })

function updateDocument(language: string) {
  document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'
}
updateDocument(i18n.language)
i18n.on('languageChanged', updateDocument)

export function setLanguage(language: Language) {
  i18n.changeLanguage(language).catch((error: unknown) => {
    console.error('Could not change language', error)
  })
  try {
    localStorage.setItem('sublane-language', language)
  } catch {
    /* Preferences still work when browser storage is unavailable. */
  }
}

declare module 'i18next' {
  interface CustomTypeOptions {
    resources: { translation: typeof en }
  }
}

export { i18n }
