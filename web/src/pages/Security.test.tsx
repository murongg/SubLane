import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { memberAuthenticated } from '@/test/fixtures'

it('enrolls a member in TOTP and displays recovery codes only in the setup dialog', async () => {
  const calls: Array<{ url: string; body: unknown }> = []
  let enabled = false
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      let data: unknown = memberAuthenticated
      if (url === '/api/me/security')
        data = {
          totp_enabled: enabled,
          recovery_codes: enabled ? 10 : 0,
          passkeys: [],
        }
      if (url === '/api/me/totp') {
        calls.push({ url, body: JSON.parse(String(init?.body)) })
        data = {
          secret: 'SYNTHETICSETUPSECRET',
          uri: 'otpauth://totp/SubLane:test?secret=SYNTHETICSETUPSECRET',
          qr: 'data:image/png;base64,aGVsbG8=',
        }
      }
      if (url === '/api/me/totp/confirm') {
        calls.push({ url, body: JSON.parse(String(init?.body)) })
        enabled = true
        data = { recovery_codes: ['ABCD-EFGH-IJKL-MNOP'] }
      }
      return Promise.resolve(new Response(JSON.stringify(data)))
    }),
  )
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/preferences'] }),
      )}
    />,
  )
  await user.click(await screen.findByRole('button', { name: 'Enable 2FA' }))
  let dialog = await screen.findByRole('dialog', {
    name: 'Enable two-factor authentication',
  })
  await user.type(
    within(dialog).getByLabelText('Current password'),
    'synthetic-password',
  )
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  expect(
    await within(dialog).findByDisplayValue('SYNTHETICSETUPSECRET'),
  ).toBeTruthy()
  expect(within(dialog).queryByDisplayValue('synthetic-password')).toBeNull()
  await user.type(within(dialog).getByLabelText('Verification code'), '123456')
  await user.click(
    within(dialog).getByRole('button', { name: 'Verify and enable' }),
  )
  dialog = await screen.findByRole('dialog', {
    name: 'Save your recovery codes',
  })
  expect(within(dialog).getByText('ABCD-EFGH-IJKL-MNOP')).toBeTruthy()
  await user.click(
    within(dialog).getByRole('button', { name: 'I saved these codes' }),
  )
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(screen.queryByText('ABCD-EFGH-IJKL-MNOP')).toBeNull()
  expect(calls).toEqual([
    {
      url: '/api/me/totp',
      body: { current_password: 'synthetic-password', code: '' },
    },
    { url: '/api/me/totp/confirm', body: { code: '123456' } },
  ])
})
