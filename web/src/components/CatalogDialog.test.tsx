import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { createQueryClient } from '@/lib/query'
import { authKey } from '@/lib/auth'
import { authenticated } from '@/test/fixtures'
import { i18n } from '@/lib/i18n'
import { CatalogDialog } from './CatalogDialog'

it.each(['account', 'group', 'key'] as const)(
  'places hidden models last in the %s catalog',
  async (kind) => {
    const visible = ['synthetic-a-visible', 'synthetic-z-visible']
    const hidden = ['synthetic-b-hidden', 'synthetic-y-hidden']
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify(
            kind === 'account'
              ? { ...snapshot, models: visible, hidden_models: hidden }
              : {
                  models: visible.map((id) => ({
                    id,
                    object: 'model',
                    owned_by: 'synthetic',
                  })),
                  hidden_models: hidden,
                  known_accounts: 1,
                  unknown_accounts: 0,
                  stale_accounts: 0,
                  refreshing: false,
                  refresh_failed: false,
                  server_time: 1900000010,
                },
          ),
        ),
      ),
    )
    const client = createQueryClient()
    client.setQueryData(authKey, authenticated)
    render(
      <QueryClientProvider client={client}>
        <CatalogDialog
          target={
            kind === 'account'
              ? { kind, id: 'synthetic-account' }
              : { kind, id: 7 }
          }
          name="Synthetic catalog"
        />
      </QueryClientProvider>,
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'Models for Synthetic catalog' }),
    )
    await screen.findByText(visible[0])
    const list = screen.getByRole('list', { name: 'Available models' })
    expect(
      within(list)
        .getAllByRole('listitem')
        .map(
          (item) =>
            item.textContent?.match(/synthetic-\w+-(?:visible|hidden)/)?.[0],
        ),
    ).toEqual([...visible, ...hidden])
    await user.type(
      screen.getByRole('textbox', { name: 'Search models' }),
      'synthetic',
    )
    expect(
      within(list)
        .getAllByRole('listitem')
        .map(
          (item) =>
            item.textContent?.match(/synthetic-\w+-(?:visible|hidden)/)?.[0],
        ),
    ).toEqual([...visible, ...hidden])
  },
)

it.each([
  ['account', 'en'],
  ['group', 'en'],
  ['key', 'en'],
  ['account', 'zh'],
  ['group', 'zh'],
  ['key', 'zh'],
] as const)(
  'labels and searches hidden models in the %s catalog in %s',
  async (kind, language) => {
    await i18n.changeLanguage(language)
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify(
            kind === 'account'
              ? {
                  ...snapshot,
                  models: [],
                  hidden_models: ['synthetic-hidden'],
                }
              : {
                  models: [],
                  hidden_models: ['synthetic-hidden'],
                  known_accounts: 1,
                  unknown_accounts: 0,
                  stale_accounts: 0,
                  refreshing: false,
                  refresh_failed: false,
                  server_time: 1900000010,
                },
          ),
        ),
      ),
    )
    const client = createQueryClient()
    client.setQueryData(authKey, authenticated)
    render(
      <QueryClientProvider client={client}>
        <CatalogDialog
          target={
            kind === 'account'
              ? { kind, id: 'synthetic-account' }
              : { kind, id: 7 }
          }
          name="Synthetic catalog"
        />
      </QueryClientProvider>,
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', {
        name: i18n.t('catalogFor', { name: 'Synthetic catalog' }),
      }),
    )
    const model = await screen.findByText('synthetic-hidden')
    const row = model.closest('li')!
    expect(
      within(row).getByText(language === 'zh' ? '隐藏模型' : 'Hidden model'),
    ).toBeTruthy()
    expect(screen.getByText(i18n.t('catalogCount', { count: 1 }))).toBeTruthy()
    const search = screen.getByRole('textbox', {
      name: i18n.t('catalogSearch'),
    })
    await user.type(search, 'unmatched')
    expect(screen.queryByText('synthetic-hidden')).toBeNull()
    expect(screen.getByText(i18n.t('catalogNoMatch'))).toBeTruthy()
    await user.clear(search)
    expect(await screen.findByText('synthetic-hidden')).toBeTruthy()
  },
)

it('inspects a pool model on demand and explains an unavailable snapshot', async () => {
  const fetcher = vi.fn((url: string) =>
    Promise.resolve(
      new Response(
        JSON.stringify(
          url.includes('/availability?')
            ? {
                model: 'synthetic-basic',
                state: 'unavailable',
                available: 0,
                unknown: 0,
                retry_at: 1900000060,
                server_time: 1900000010,
                reasons: [{ code: 'quota_exhausted', count: 1 }],
                accounts: [
                  {
                    id: 'synthetic-account',
                    name: 'Synthetic subscription',
                    provider: 'codex',
                    reason: 'quota_exhausted',
                    quota_state: 'exhausted',
                    retry_at: 1900000060,
                    in_flight: 0,
                    max_concurrency: 10,
                  },
                ],
              }
            : {
                models: [
                  {
                    id: 'synthetic-basic',
                    object: 'model',
                    owned_by: 'openai',
                  },
                ],
                known_accounts: 1,
                unknown_accounts: 0,
                stale_accounts: 0,
                refreshing: false,
                refresh_failed: false,
                server_time: 1900000010,
              },
        ),
      ),
    ),
  )
  vi.stubGlobal('fetch', fetcher)
  const client = createQueryClient()
  client.setQueryData(authKey, authenticated)
  render(
    <QueryClientProvider client={client}>
      <CatalogDialog target={{ kind: 'group', id: 7 }} name="Synthetic pool" />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic pool' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'Inspect synthetic-basic' }),
  )
  expect(await screen.findByText('No eligible account')).toBeTruthy()
  expect(screen.getByText('Synthetic subscription')).toBeTruthy()
  expect(
    fetcher.mock.calls.filter(([url]) =>
      String(url).includes('/availability?'),
    ),
  ).toHaveLength(1)
})

const snapshot = {
  models: ['synthetic-basic'],
  updated_at: 1900000000,
  server_time: 1900000010,
  known: true,
  usable: true,
  stale: false,
  refreshing: false,
  refresh_failed: false,
  retry_after_seconds: 0,
}
function mount(kind: 'account' | 'channel' = 'account') {
  const client = createQueryClient()
  client.setQueryData(authKey, authenticated)
  render(
    <QueryClientProvider client={client}>
      <CatalogDialog
        target={{ kind, id: 'synthetic-account' }}
        name="Synthetic subscription"
      />
    </QueryClientProvider>,
  )
  return client
}
it.each(['account', 'channel'] as const)(
  'tests a selected %s model only after clicking and shows the result',
  async (kind) => {
    let resolve!: (value: Response) => void
    const fetcher = vi.fn((url: string) =>
      url.endsWith('/check')
        ? new Promise<Response>((done) => {
            resolve = done
          })
        : Promise.resolve(new Response(JSON.stringify(snapshot))),
    )
    vi.stubGlobal('fetch', fetcher)
    const user = userEvent.setup()
    mount(kind)
    await user.click(
      screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
    )
    const button = await screen.findByRole('button', {
      name: 'Test synthetic-basic',
    })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(
      screen.getByText(/uses a small amount of upstream quota/),
    ).toBeTruthy()
    await user.click(button)
    expect(
      screen
        .getByRole('button', { name: 'Testing synthetic-basic…' })
        .hasAttribute('disabled'),
    ).toBe(true)
    expect(fetcher).toHaveBeenLastCalledWith(
      `/api/${kind === 'account' ? 'accounts' : 'channels'}/synthetic-account/models/check`,
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ model: 'synthetic-basic' }),
      }),
    )
    await act(async () =>
      resolve(
        new Response(
          JSON.stringify({
            model: 'synthetic-basic',
            available: true,
            duration_ms: 123,
            error_code: '',
          }),
        ),
      ),
    )
    expect(
      await screen.findByText('Responded successfully · 123 ms'),
    ).toBeTruthy()
    expect(button.hasAttribute('disabled')).toBe(false)
  },
)

it('explains a failed model check and allows retrying', async () => {
  const fetcher = vi.fn((url: string) =>
    Promise.resolve(
      new Response(
        JSON.stringify(
          url.endsWith('/check')
            ? {
                model: 'synthetic-basic',
                available: false,
                duration_ms: 42,
                error_code: 'rate_limited',
              }
            : snapshot,
        ),
      ),
    ),
  )
  vi.stubGlobal('fetch', fetcher)
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'Test synthetic-basic' }),
  )
  expect(await screen.findByText('Rate limited · 42 ms')).toBeTruthy()
  expect(screen.queryByText(/Responded successfully/)).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Test synthetic-basic' }))
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3))
})

it('cancels a model check when its dialog closes', async () => {
  let signal: AbortSignal | undefined
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith('/check')) {
      signal = init?.signal as AbortSignal
      return new Promise<Response>(() => {})
    }
    return Promise.resolve(new Response(JSON.stringify(snapshot)))
  })
  vi.stubGlobal('fetch', fetcher)
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'Test synthetic-basic' }),
  )
  await user.keyboard('{Escape}')
  await waitFor(() => expect(signal?.aborted).toBe(true))
})

it('explains a rejected check without losing the model list', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        url.endsWith('/check')
          ? new Response(JSON.stringify({ error: 'gateway_busy' }), {
              status: 429,
            })
          : new Response(JSON.stringify(snapshot)),
      ),
    ),
  )
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'Test synthetic-basic' }),
  )
  expect(
    await screen.findByText(
      'The gateway is busy. Wait a moment and try again.',
    ),
  ).toBeTruthy()
  expect(screen.getByText('synthetic-basic')).toBeTruthy()
  expect(
    screen
      .getByRole('button', { name: 'Test synthetic-basic' })
      .hasAttribute('disabled'),
  ).toBe(false)
})
it('loads on demand and keeps the last successful catalog after a refresh failure', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(snapshot)))
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  mount()
  expect(fetch).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  expect(await screen.findByText('synthetic-basic')).toBeTruthy()
  fetch.mockResolvedValueOnce(
    new Response(
      JSON.stringify({
        ...snapshot,
        stale: true,
        refresh_failed: true,
        retry_after_seconds: 30,
      }),
    ),
  )
  await user.click(screen.getByRole('button', { name: 'Refresh models' }))
  expect(
    await screen.findByText('Refresh failed. Showing the last saved catalog.'),
  ).toBeTruthy()
  expect(screen.getByText('synthetic-basic')).toBeTruthy()
})
it('shows unknown capability without claiming the account supports no models', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          models: [],
          updated_at: 0,
          known: false,
          usable: false,
          stale: true,
          refresh_failed: true,
        }),
      ),
    ),
  )
  const user = userEvent.setup()
  mount()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  expect(
    await screen.findByText(
      'Model support is unknown. Refresh to retrieve the account catalog.',
    ),
  ).toBeTruthy()
  expect(
    screen.queryByText('This account reported no visible models.'),
  ).toBeNull()
})

it('ignores a late refresh failure after the active identity changes', async () => {
  let resolve!: (value: Response) => void
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(snapshot)))
    .mockImplementation(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  const client = mount()
  await user.click(
    screen.getByRole('button', { name: 'Models for Synthetic subscription' }),
  )
  await screen.findByText('synthetic-basic')
  await user.click(screen.getByRole('button', { name: 'Refresh models' }))
  await act(async () => {
    client.setQueryData(authKey, {
      ...authenticated,
      user: { ...authenticated.user, id: 42, role: 'member' },
    })
    resolve(
      new Response(JSON.stringify({ error: 'unauthorized' }), { status: 401 }),
    )
  })
  await waitFor(() => expect(client.isMutating()).toBe(0))
  expect(client.getQueryData<typeof authenticated>(authKey)?.user?.id).toBe(42)
})
