import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { authKey } from '@/lib/auth'
import { createQueryClient } from '@/lib/query'
import { authenticated } from '@/test/fixtures'
import { TurnstileSettings } from './TurnstileSettings'

it('saves keys without redisplaying the secret and preserves it on subsequent saves', async () => {
  const client = createQueryClient()
  client.setQueryData(authKey, authenticated)
  const bodies: Record<string, unknown>[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') {
        bodies.push(JSON.parse(String(init.body)))
        return Promise.resolve(
          new Response(
            JSON.stringify({
              enabled: true,
              site_key: 'synthetic-site',
              secret_configured: true,
            }),
          ),
        )
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            enabled: false,
            site_key: '',
            secret_configured: false,
          }),
        ),
      )
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <TurnstileSettings userID={1} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Site Key'), 'synthetic-site')
  await user.type(screen.getByLabelText('Secret Key'), 'synthetic-secret')
  await user.click(
    screen.getByRole('checkbox', { name: 'Require verification at sign-in' }),
  )
  await user.click(screen.getByRole('button', { name: 'Save Turnstile' }))
  await screen.findByText('Turnstile settings saved.')
  expect(bodies[0]).toEqual({
    enabled: true,
    site_key: 'synthetic-site',
    secret_key: 'synthetic-secret',
  })
  expect((screen.getByLabelText('Secret Key') as HTMLInputElement).value).toBe(
    '',
  )
  await user.click(screen.getByRole('button', { name: 'Save Turnstile' }))
  expect(bodies[1]).toEqual({ enabled: true, site_key: 'synthetic-site' })
})
