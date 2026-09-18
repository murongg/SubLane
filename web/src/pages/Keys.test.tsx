import { act, render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authOptions } from '@/lib/auth'
import * as queries from '@/lib/query'
import { memberAuthenticated } from '@/test/fixtures'

const secret = 'sl_' + 'x'.repeat(43)
const metadata = {
  id: 1,
  name: 'Synthetic laptop',
  prefix: 'sl_xxxxxxxx',
  created_at: 1900000000,
  last_used_at: null,
  revoked_at: null as number | null,
}
const response = (value: unknown) => new Response(JSON.stringify(value))
function open() {
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/keys'] }),
      )}
    />,
  )
}

it('lets a member create, copy once, and revoke an owned key', async () => {
  let keys: (typeof metadata)[] = []
  const fetchMock = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(memberAuthenticated))
      if (url.endsWith('/revoke')) {
        keys = [{ ...metadata, revoked_at: 1900000100 }]
        return Promise.resolve(response(keys[0]))
      }
      if (init?.method === 'POST') {
        keys = [metadata]
        return Promise.resolve(response({ key: metadata, secret }))
      }
      return Promise.resolve(response({ keys, next_cursor: 0 }))
    })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  open()
  await screen.findByText('No API keys yet')
  await user.click(screen.getByRole('button', { name: 'Create key' }))
  const form = await screen.findByRole('dialog', { name: 'Create API key' })
  await user.type(within(form).getByLabelText('Name'), 'Synthetic laptop')
  await user.click(within(form).getByRole('button', { name: 'Create key' }))
  const created = await screen.findByRole('dialog', {
    name: 'Save your API key',
  })
  expect(
    (within(created).getByLabelText('API key') as HTMLInputElement).value,
  ).toBe(secret)
  await user.click(within(created).getByRole('button', { name: 'Copy key' }))
  await waitFor(() => expect(copy).toHaveBeenCalledWith(secret))
  await user.click(within(created).getByRole('button', { name: 'Done' }))
  expect(screen.queryByDisplayValue(secret)).toBeNull()
  await user.click(
    await screen.findByRole('button', { name: 'Revoke Synthetic laptop' }),
  )
  const revoke = await screen.findByRole('dialog', { name: 'Revoke API key' })
  await user.click(within(revoke).getByRole('button', { name: 'Revoke key' }))
  expect(await screen.findByText('Revoked')).toBeTruthy()
  expect(
    fetchMock.mock.calls.some(
      ([url]) => url === '/api/system' || url.startsWith('/api/members'),
    ),
  ).toBe(false)
})

it('discards a displayed secret when a background check changes the user', async () => {
  const client = queries.createQueryClient()
  vi.spyOn(queries, 'createQueryClient').mockReturnValue(client)
  let state = memberAuthenticated
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state') return Promise.resolve(response(state))
      if (init?.method === 'POST')
        return Promise.resolve(response({ key: metadata, secret }))
      return Promise.resolve(response({ keys: [], next_cursor: 0 }))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('No API keys yet')
  await user.click(screen.getByRole('button', { name: 'Create key' }))
  const form = await screen.findByRole('dialog', { name: 'Create API key' })
  await user.type(within(form).getByLabelText('Name'), 'Synthetic laptop')
  await user.click(within(form).getByRole('button', { name: 'Create key' }))
  await screen.findByDisplayValue(secret)
  state = {
    ...memberAuthenticated,
    user: { ...memberAuthenticated.user, id: 3, username: 'another-test' },
  }
  await act(async () => {
    await client.fetchQuery({ ...authOptions(), staleTime: 0 })
  })
  await waitFor(() => expect(screen.queryByDisplayValue(secret)).toBeNull())
  expect(screen.queryByRole('dialog')).toBeNull()
  await waitFor(() =>
    expect(client.getMutationCache().getAll()).toHaveLength(0),
  )
  client.clear()
})
