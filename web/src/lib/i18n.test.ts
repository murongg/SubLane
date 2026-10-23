import { expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/react'

const loadChinese = vi.hoisted(() => vi.fn())
vi.mock('@/locales/zh', async (original) => {
  loadChinese()
  return original()
})

it('does not load the Chinese dictionary for an English session', async () => {
  localStorage.setItem('sublane-language', 'en')
  vi.resetModules()
  loadChinese.mockClear()
  const { i18n } = await import('./i18n')
  await waitFor(() => expect(i18n.isInitialized).toBe(true))
  expect(i18n.resolvedLanguage).toBe('en')
  expect(loadChinese).not.toHaveBeenCalled()
})

it.each(['zh', 'ja', 'zh-TW', 'ar'])(
  'restores %s before completing initialization',
  async (language) => {
    localStorage.setItem('sublane-language', language)
    vi.resetModules()
    const { i18n } = await import('./i18n')
    await waitFor(() => expect(i18n.resolvedLanguage).toBe(language))
    expect(document.documentElement.lang).toBe(
      language === 'zh' ? 'zh-CN' : language,
    )
    expect(document.documentElement.dir).toBe(language === 'ar' ? 'rtl' : 'ltr')
  },
)

it('falls back to English for an unsupported stored language', async () => {
  localStorage.setItem('sublane-language', 'not-a-language')
  vi.resetModules()
  const { i18n } = await import('./i18n')
  if (!i18n.isInitialized)
    await new Promise((resolve) => i18n.on('initialized', resolve))
  expect(i18n.resolvedLanguage).toBe('en')
})
