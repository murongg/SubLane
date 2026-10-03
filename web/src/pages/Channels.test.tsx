import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import * as queries from '@/lib/query'
import { authenticated } from '@/test/fixtures'

const channel = {
  id: 'synthetic-channel',
  name: 'Synthetic API',
  protocol: 'openai',
  base_url: 'https://relay.example.test/v1',
  enabled: true,
  status: 'unverified',
  max_concurrency: 30,
  created_at: 1,
  updated_at: 1,
}
const response = (value: unknown) => new Response(JSON.stringify(value))

it('creates a channel independently and removes the key from mutation state', async () => {
  let saved = false
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/proxies')
        return Promise.resolve(response({ proxies: [] }))
      if (url === '/api/channels' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({
          name: 'Synthetic API',
          api_key: 'synthetic-key',
          base_url: 'https://relay.example.test/v1',
        })
        saved = true
        return Promise.resolve(response(channel))
      }
      if (url === '/api/channels')
        return Promise.resolve(response({ channels: saved ? [channel] : [] }))
      if (url === '/api/channels/runtime')
        return Promise.resolve(response({ channels: [], server_time: 1 }))
      return Promise.resolve(response({}))
    })
  vi.stubGlobal('fetch', fetch)
  const client = queries.createQueryClient()
  vi.spyOn(queries, 'createQueryClient').mockReturnValue(client)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/channels'] }),
      )}
    />,
  )
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'API channels' })
  await user.click(screen.getByRole('button', { name: 'Add API channel' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText('Channel name'),
    'Synthetic API',
  )
  await user.clear(within(dialog).getByLabelText('Base URL'))
  await user.type(
    within(dialog).getByLabelText('Base URL'),
    'https://relay.example.test/v1',
  )
  const key = within(dialog).getByLabelText('Upstream API key')
  expect(key.getAttribute('type')).toBe('password')
  await user.type(key, 'synthetic-key')
  await user.click(within(dialog).getByRole('button', { name: 'Connect API' }))
  await screen.findByText('Synthetic API')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await waitFor(() =>
    expect(client.getMutationCache().getAll()).toHaveLength(0),
  )
  expect(
    fetch.mock.calls.some(([url]) => String(url).startsWith('/api/accounts')),
  ).toBe(false)
})

it('replaces a channel key without exposing the stored key or changing its endpoint', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/channels')
        return Promise.resolve(response({ channels: [channel] }))
      if (url === '/api/channels/runtime')
        return Promise.resolve(response({ channels: [], server_time: 1 }))
      if (url === '/api/channels/synthetic-channel/key') {
        expect(JSON.parse(String(init?.body))).toEqual({
          name: channel.name,
          base_url: channel.base_url,
          api_key: 'synthetic-rotated-key',
        })
        return Promise.resolve(response(channel))
      }
      return Promise.resolve(response({}))
    }),
  )
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/channels'] }),
      )}
    />,
  )
  const user = userEvent.setup()
  await screen.findByText('Synthetic API')
  await user.click(
    screen.getByRole('button', { name: 'Actions for Synthetic API' }),
  )
  await user.click(screen.getByRole('menuitem', { name: 'Replace API key' }))
  const dialog = await screen.findByRole('dialog')
  expect(
    (within(dialog).getByLabelText('Base URL') as HTMLInputElement).readOnly,
  ).toBe(true)
  expect(
    (within(dialog).getByLabelText('Upstream API key') as HTMLInputElement)
      .value,
  ).toBe('')
  await user.type(
    within(dialog).getByLabelText('Upstream API key'),
    'synthetic-rotated-key',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Replace API key' }),
  )
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
})
