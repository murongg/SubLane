import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { createQueryClient } from '@/lib/query'
import { authKey } from '@/lib/auth'
import type { APIKey } from '@/lib/keys'
import * as magpie from '@/lib/magpie'
import { memberAuthenticated } from '@/test/fixtures'
import { MagpieImport } from './MagpieImport'

const secret = 'sl_' + 'x'.repeat(43)
const key: APIKey = {
  id: 1,
  name: 'Synthetic laptop',
  prefix: 'sl_xxxxxxxx',
  group_id: 2,
  group_name: 'Synthetic group',
  group_access: 'allowed',
  enabled: true,
  copyable: true,
  created_at: 1900000000,
  last_used_at: null,
  revoked_at: null,
  expires_at: null,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })

function open(value = key, usable = true) {
  const client = createQueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  const view = render(
    <QueryClientProvider client={client}>
      <MagpieImport value={value} userID={2} usable={usable} />
    </QueryClientProvider>,
  )
  return { ...view, client }
}

it('retrieves the key on request and opens Magpie with a separate user gesture', async () => {
  const fetch = vi.fn().mockResolvedValue(response({ secret }))
  vi.stubGlobal('fetch', fetch)
  const launch = vi.spyOn(magpie, 'openMagpie').mockImplementation(() => {})
  const user = userEvent.setup()
  const { client } = open()
  const trigger = screen.getByRole('button', {
    name: 'Import Synthetic laptop into Magpie',
  })
  await user.click(trigger)
  expect(fetch).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  const ready = await screen.findByRole('button', { name: 'Open Magpie' })
  expect(fetch).toHaveBeenCalledExactlyOnceWith(
    '/api/keys/1/secret',
    expect.objectContaining({ method: 'POST', cache: 'no-store' }),
  )
  expect(launch).not.toHaveBeenCalled()
  expect(document.documentElement.innerHTML).not.toContain(secret)
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data),
    ),
  ).not.toContain(secret)
  expect(
    JSON.stringify(
      client
        .getMutationCache()
        .getAll()
        .map((m) => m.state),
    ),
  ).not.toContain(secret)
  await user.click(ready)
  const link = new URL(launch.mock.calls[0][0])
  expect(link.searchParams.get('key')).toBe(secret)
  expect(link.searchParams.get('responses')).toBe(
    window.location.origin + '/v1',
  )
  expect(link.searchParams.has('models')).toBe(false)
  await user.click(screen.getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(document.activeElement).toBe(trigger)
  await user.click(trigger)
  expect(screen.queryByRole('button', { name: 'Open Magpie' })).toBeNull()
})

it.each([
  { copyable: false },
  { enabled: false },
  { revoked_at: 1900000000 },
  { group_access: 'blocked' as const },
  { expires_at: 1900000000 },
])('blocks unavailable keys: %j', (changes) => {
  open({ ...key, ...changes }, !changes.expires_at)
  expect(
    screen
      .getByRole('button', {
        name: 'Import Synthetic laptop into Magpie',
      })
      .hasAttribute('disabled'),
  ).toBe(true)
})

it('reports disclosure and launch failures and allows retrying', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response({ error: 'api_key_not_copyable' }, 409))
    .mockResolvedValue(response({ secret }))
  vi.stubGlobal('fetch', fetch)
  const launch = vi.spyOn(magpie, 'openMagpie').mockImplementation(() => {
    throw new Error('synthetic_launch_failure')
  })
  const user = userEvent.setup()
  open()
  await user.click(
    screen.getByRole('button', { name: 'Import Synthetic laptop into Magpie' }),
  )
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'This older key cannot be copied again.',
  )
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  await user.click(await screen.findByRole('button', { name: 'Open Magpie' }))
  expect(screen.getByRole('alert').textContent).toContain(
    'Could not open Magpie.',
  )
  launch.mockImplementation(() => {})
  await user.click(screen.getByRole('button', { name: 'Open Magpie' }))
  expect(screen.queryByRole('alert')).toBeNull()
})

it('aborts a pending disclosure when closed', async () => {
  let resolve!: (value: Response) => void
  let signal: AbortSignal | undefined | null
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init?: RequestInit) => {
      signal = init?.signal
      return new Promise<Response>((done) => {
        resolve = done
      })
    }),
  )
  const user = userEvent.setup()
  open()
  const trigger = screen.getByRole('button', {
    name: 'Import Synthetic laptop into Magpie',
  })
  await user.click(trigger)
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(signal?.aborted).toBe(true))
  await act(async () => {
    resolve(response({ secret }))
  })
  await user.click(trigger)
  expect(screen.queryByRole('button', { name: 'Open Magpie' })).toBeNull()
})

it('discards a prepared link when the key becomes unavailable and guards identity changes', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ secret })))
  const launch = vi.spyOn(magpie, 'openMagpie').mockImplementation(() => {})
  const user = userEvent.setup()
  const view = open()
  await user.click(
    screen.getByRole('button', { name: 'Import Synthetic laptop into Magpie' }),
  )
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  const ready = await screen.findByRole('button', { name: 'Open Magpie' })
  await act(async () => {
    view.client.setQueryData(authKey, {
      ...memberAuthenticated,
      user: { ...memberAuthenticated.user, id: 3 },
    })
  })
  await user.click(ready)
  expect(launch).not.toHaveBeenCalled()
  view.rerender(
    <QueryClientProvider client={view.client}>
      <MagpieImport
        value={{ ...key, enabled: false }}
        userID={2}
        usable={false}
      />
    </QueryClientProvider>,
  )
  expect(screen.queryByRole('button', { name: 'Open Magpie' })).toBeNull()
})
