import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'
import { i18n } from '@/lib/i18n'
import { LanguageSelect } from './Preferences'

it.each([
  ['zh-TW', '繁體中文'],
  ['ja', '日本語'],
  ['ko', '한국어'],
  ['es', 'Español'],
  ['fr', 'Français'],
  ['de', 'Deutsch'],
  ['pt', 'Português'],
  ['ru', 'Русский'],
  ['ar', 'العربية'],
])(
  'switches to %s, persists it and updates document language and direction',
  async (language, label) => {
    const user = userEvent.setup()
    render(<LanguageSelect />)
    await user.click(screen.getByRole('button', { name: 'Language' }))
    await user.click(screen.getByRole('menuitemradio', { name: label }))
    await waitFor(() => expect(i18n.resolvedLanguage).toBe(language))
    expect(localStorage.getItem('sublane-language')).toBe(language)
    expect(document.documentElement.lang).toBe(language)
    expect(document.documentElement.dir).toBe(language === 'ar' ? 'rtl' : 'ltr')
    await act(async () => {
      await i18n.changeLanguage('en')
    })
    expect(document.documentElement.dir).toBe('ltr')
  },
)
