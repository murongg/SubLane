import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import * as queries from '@/lib/query'
import { authenticated } from '@/test/fixtures'

const account = {
  id: 'synthetic-account',
  name: 'Test subscription',
  email: 'member@example.test',
  plan: 'plus',
  enabled: true,
  status: 'unverified',
  expires_at: 0,
  created_at: 1,
  updated_at: 1,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })
function open() {
  const client = queries.createQueryClient()
  vi.spyOn(queries, 'createQueryClient').mockReturnValue(client)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/accounts'] }),
      )}
    />,
  )
  return client
}

it('imports a subscription and clears credential input after success', async () => {
  let accounts: (typeof account)[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/import' && init?.method === 'POST') {
        accounts = [account]
        return Promise.resolve(response(account, 201))
      }
      return Promise.resolve(response({ accounts }))
    }),
  )
  const user = userEvent.setup()
  const client = open()
  await screen.findByText('No subscription accounts')
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText('Account name'),
    'Test subscription',
  )
  await user.click(
    within(dialog).getByRole('button', {
      name: 'Import auth.json',
    }),
  )
  await user.click(within(dialog).getByLabelText('auth.json contents'))
  await user.paste(
    '{"tokens":{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"upstream-test"}}',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Import account' }),
  )
  await screen.findByText('Test subscription')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(screen.queryByDisplayValue(/synthetic-access/)).toBeNull()
  await waitFor(() =>
    expect(client.getMutationCache().getAll()).toHaveLength(0),
  )
})

it('starts browser authorization and submits only the callback for the matching attempt', async () => {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push(url)
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/oauth')
        return Promise.resolve(
          response(
            {
              url: 'https://auth.openai.com/oauth/authorize?state=synthetic-state',
              state: 'synthetic-state',
              expires_at: 9999999999,
            },
            201,
          ),
        )
      if (url === '/api/accounts/oauth/complete') {
        expect(JSON.parse(String(init?.body))).toEqual({
          state: 'synthetic-state',
          callback_url:
            'http://localhost:1455/auth/callback?state=synthetic-state&code=synthetic-code',
        })
        return Promise.resolve(response({ ...account, status: 'ready' }, 201))
      }
      return Promise.resolve(response({ accounts: [] }))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('No subscription accounts')
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText('Account name'),
    'Test subscription',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Start authorization' }),
  )
  await within(dialog).findByRole('link', {
    name: 'Open provider authorization',
  })
  await user.type(
    within(dialog).getByLabelText('Callback URL'),
    'http://localhost:1455/auth/callback?state=synthetic-state&code=synthetic-code',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Complete authorization' }),
  )
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(calls).toContain('/api/accounts/oauth/complete')
})

it('keeps quota, identity and accessible actions together for each account', async () => {
  const calls: string[] = []
  const now = Math.floor(Date.now() / 1000)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      calls.push(url)
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url.endsWith('/check'))
        return Promise.resolve(response({ account, models: [] }))
      if (url.endsWith('/usage'))
        return Promise.resolve(
          response({
            updated_at: now,
            server_time: now,
            expires_at: now + 120,
            stale: false,
            refreshing: false,
            refresh_failed: false,
            retry_after_seconds: 0,
            limits: [
              {
                name: '',
                allowed: true,
                limit_reached: false,
                windows: [
                  {
                    kind: 'secondary',
                    used_percent: 21,
                    window_seconds: 604800,
                    reset_at: now + 3600,
                  },
                ],
              },
            ],
          }),
        )
      return Promise.resolve(response({ accounts: [account] }))
    }),
  )
  open()
  const row = await screen.findByRole('article', { name: 'Test subscription' })
  expect(within(row).getByText('member@example.test')).toBeTruthy()
  await within(row).findByText('79% remaining')
  expect(
    within(row).getByRole('button', {
      name: 'Refresh usage for Test subscription',
    }),
  ).toBeTruthy()
  await userEvent.setup().click(
    within(row).getByRole('button', {
      name: 'Verify connection for Test subscription',
    }),
  )
  await waitFor(() =>
    expect(calls).toContain('/api/accounts/synthetic-account/check'),
  )
  expect(
    within(row).getByRole('button', { name: 'Actions for Test subscription' }),
  ).toBeTruthy()
})

it('chooses a provider and starts its authorization without reusing Codex URLs', async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/oauth') {
        expect(JSON.parse(String(init?.body)).provider).toBe('claude')
        return Promise.resolve(
          response(
            {
              url: 'https://claude.ai/oauth/authorize?state=synthetic-state',
              state: 'synthetic-state',
              expires_at: 9999999999,
              callback_url: 'http://localhost:54545/callback',
            },
            201,
          ),
        )
      }
      return Promise.resolve(response({ accounts: [] }))
    })
  vi.stubGlobal('fetch', fetch)
  open()
  await screen.findByText('No subscription accounts')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const choices = screen.getByRole('group', { name: 'Service provider' })
  expect(within(choices).getAllByRole('button')).toHaveLength(3)
  expect(
    within(choices).getByRole('button', { name: 'Codex', pressed: true }),
  ).toBeTruthy()
  await user.click(within(choices).getByRole('button', { name: 'Claude' }))
  expect(
    within(choices).getByRole('button', { name: 'Claude', pressed: true }),
  ).toBeTruthy()
  await user.type(screen.getByLabelText('Account name'), 'Synthetic Claude')
  await user.click(screen.getByRole('button', { name: 'Start authorization' }))
  expect(
    (
      await screen.findByRole('link', { name: 'Open provider authorization' })
    ).getAttribute('href'),
  ).toContain('claude.ai/oauth/authorize')
  expect(
    screen.getByPlaceholderText('http://localhost:54545/callback?...'),
  ).toBeTruthy()
  for (const button of within(choices).getAllByRole('button')) {
    expect((button as HTMLButtonElement).disabled).toBe(true)
  }
})

it('updates an account concurrency limit from scheduling settings', async () => {
  let limit = 2
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url.endsWith('/limits')) {
        limit = JSON.parse(String(init?.body)).max_concurrency
        return Promise.resolve(response({ ...account, max_concurrency: limit }))
      }
      if (url === '/api/accounts/runtime')
        return Promise.resolve(
          response({
            accounts: [
              {
                id: account.id,
                max_concurrency: limit,
                in_flight: 0,
                cooldown_until: 0,
                reason: '',
                failures: 0,
                state: 'available',
              },
            ],
            server_time: 1900000000,
          }),
        )
      return Promise.resolve(
        response({
          accounts: [{ ...account, enabled: false, max_concurrency: limit }],
        }),
      )
    })
  vi.stubGlobal('fetch', fetch)
  open()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', {
      name: 'Actions for Test subscription',
    }),
  )
  await user.click(
    screen.getByRole('menuitem', { name: 'Scheduling settings' }),
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Scheduling settings',
  })
  const input = within(dialog).getByLabelText('Concurrent model requests')
  await user.clear(input)
  await user.type(input, '1')
  await user.click(
    within(dialog).getByRole('button', { name: 'Save settings' }),
  )
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(limit).toBe(1)
})
