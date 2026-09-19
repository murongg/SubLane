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
  group_id: 1,
  group_name: 'Default',
  group_access: 'allowed',
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
      if (url === '/api/keys/groups')
        return Promise.resolve(
          response({ groups: [{ id: 1, name: 'Default' }] }),
        )
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
      if (url === '/api/keys/groups')
        return Promise.resolve(
          response({ groups: [{ id: 1, name: 'Default' }] }),
        )
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

it('binds a member key to the chosen authorized group', async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(memberAuthenticated))
      if (url === '/api/keys/groups')
        return Promise.resolve(
          response({
            groups: [
              { id: 2, name: 'Project alpha' },
              { id: 3, name: 'Project beta' },
            ],
          }),
        )
      if (url === '/api/keys' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({
          name: 'Synthetic project key',
          group_id: 3,
        })
        return Promise.resolve(
          response({
            key: { ...metadata, group_id: 3, group_name: 'Project beta' },
            secret,
          }),
        )
      }
      if (url === '/api/connection')
        return Promise.resolve(response({ status: 'ready' }))
      return Promise.resolve(response({ keys: [], next_cursor: 0 }))
    })
  vi.stubGlobal('fetch', fetch)
  open()
  const user = userEvent.setup()
  await screen.findByText('No API keys yet')
  await user.click(screen.getByRole('button', { name: 'Create key' }))
  const dialog = await screen.findByRole('dialog', { name: 'Create API key' })
  await within(dialog).findByText('Project alpha')
  await user.click(
    within(dialog).getByRole('button', { name: 'Account group' }),
  )
  await user.click(screen.getByRole('menuitemradio', { name: 'Project beta' }))
  await user.type(
    within(dialog).getByLabelText('Name'),
    'Synthetic project key',
  )
  await user.click(within(dialog).getByRole('button', { name: 'Create key' }))
  await screen.findByDisplayValue(secret)
})

it('opens client setup on demand, copies the model config, and restores focus on close', async () => {
  const fetch = vi.fn().mockImplementation((url: string) => {
    if (url === '/api/auth/state')
      return Promise.resolve(response(memberAuthenticated))
    if (url === '/api/connection')
      return Promise.resolve(response({ status: 'ready' }))
    return Promise.resolve(response({ keys: [], next_cursor: 0 }))
  })
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  open()
  await screen.findByText('No API keys yet')
  expect(fetch.mock.calls.some(([url]) => url === '/api/connection')).toBe(
    false,
  )
  const trigger = screen.getByRole('button', { name: 'Setup guide' })
  await user.click(trigger)
  const guide = await screen.findByRole('dialog', {
    name: 'Connect your client',
  })
  await within(guide).findByText('Ready')
  await user.type(
    within(guide).getByLabelText('Model ID'),
    'codex/synthetic-model',
  )
  await user.click(
    within(guide).getByRole('button', { name: 'Copy configuration' }),
  )
  await waitFor(() =>
    expect(copy).toHaveBeenCalledWith(
      expect.stringContaining('model = "codex/synthetic-model"'),
    ),
  )
  expect(copy.mock.calls[0][0]).toContain('env_key = "SUBLANE_API_KEY"')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(document.activeElement).toBe(trigger)
  await user.click(trigger)
  expect(
    ((await screen.findByLabelText('Model ID')) as HTMLInputElement).value,
  ).toBe('codex/synthetic-model')
})
