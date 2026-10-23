import i18n from 'i18next'
import type { ReadCallback } from 'i18next'
import { initReactI18next } from 'react-i18next'
import { en } from '@/locales/en'

export const languages = [
  { value: 'en', label: 'English' },
  { value: 'zh', label: '简体中文' },
  { value: 'zh-TW', label: '繁體中文' },
  { value: 'ja', label: '日本語' },
  { value: 'ko', label: '한국어' },
  { value: 'es', label: 'Español' },
  { value: 'fr', label: 'Français' },
  { value: 'de', label: 'Deutsch' },
  { value: 'pt', label: 'Português' },
  { value: 'ru', label: 'Русский' },
  { value: 'ar', label: 'العربية' },
] as const

export type Language = (typeof languages)[number]['value']

const loaders = {
  zh: () => import('@/locales/zh').then(({ zh }) => ({ default: zh })),
  'zh-TW': () => import('@/locales/zh-TW.json'),
  ja: () => import('@/locales/ja.json'),
  ko: () => import('@/locales/ko.json'),
  es: () => import('@/locales/es.json'),
  fr: () => import('@/locales/fr.json'),
  de: () => import('@/locales/de.json'),
  pt: () => import('@/locales/pt.json'),
  ru: () => import('@/locales/ru.json'),
  ar: () => import('@/locales/ar.json'),
} satisfies Record<
  Exclude<Language, 'en'>,
  () => Promise<{ default: Record<keyof typeof en, string> }>
>

function savedLanguage(): Language {
  try {
    const saved = localStorage.getItem('sublane-language')
    return languages.find(({ value }) => value === saved)?.value ?? 'en'
  } catch {
    return 'en'
  }
}

i18n
  .use({
    type: 'backend' as const,
    read(language: string, _namespace: string, callback: ReadCallback) {
      const load = loaders[language as keyof typeof loaders]
      if (!load) {
        callback(new Error(`Unsupported language: ${language}`), false)
        return
      }
      load()
        .then(({ default: translation }) => callback(null, translation))
        .catch((error: unknown) => callback(error as Error, false))
    },
  })
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en } },
    lng: savedLanguage(),
    fallbackLng: 'en',
    supportedLngs: languages.map(({ value }) => value),
    // Traditional Chinese must not resolve to the Simplified Chinese dictionary.
    load: 'currentOnly',
    partialBundledLanguages: true,
    interpolation: { escapeValue: false },
  })
  .catch((error: unknown) => {
    console.error('Could not initialize translations', error)
  })

function updateDocument(language = 'en') {
  document.documentElement.lang = language === 'zh' ? 'zh-CN' : language
  document.documentElement.dir = i18n.dir(language)
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
