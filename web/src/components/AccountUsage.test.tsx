import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { AccountUsage } from './AccountUsage'
import type { Provider } from '@/lib/accounts'
import type { UsageWindow } from '@/lib/usage'

const now = Math.floor(Date.now() / 1000)
const metadata = {
  server_time: now,
  expires_at: now + 120,
  stale: false,
  refreshing: false,
  refresh_failed: false,
  refresh_error: '',
  retry_after_seconds: 0,
}
const snapshot = {
  ...metadata,
  updated_at: now,
  limits: [
    {
      name: '',
      allowed: true,
      limit_reached: false,
      windows: [
        {
          kind: 'primary',
          used_percent: 25,
          window_seconds: 18000,
          reset_at: now + 7200,
        },
        {
          kind: 'secondary',
          used_percent: 100,
          window_seconds: 604800,
          reset_at: now + 86400,
        },
      ],
    },
  ],
}
function mount(provider: Provider = 'codex') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccountUsage
        id="synthetic-account"
        name="Test subscription"
        provider={provider}
      />
    </QueryClientProvider>,
  )
  return client
}

it('compares usage with even pace at the observation time and explains the reference', async () => {
  const observed = now - 300
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          updated_at: observed,
          limits: [
            {
              ...snapshot.limits[0],
              windows: [
                {
                  kind: 'primary',
                  used_percent: 70,
                  window_seconds: 3600,
                  reset_at: observed + 1800,
                },
              ],
            },
          ],
        }),
      ),
    ),
  )
  mount()
  const pace = await screen.findByText(
    '20 percentage points ahead of even pace',
  )
  expect(screen.getByText('30% remaining').className).toContain('text-success')
  const disclosure = pace.closest('details')
  expect(disclosure?.open).toBe(false)
  const summary = pace.closest('summary')
  expect(summary).toBeTruthy()
  await userEvent.setup().click(summary!)
  expect(disclosure?.open).toBe(true)
  expect(screen.getByText(/at the last usage observation/)).toBeTruthy()
})

it.each([
  [30, '20 percentage points behind even pace'],
  [50, 'On even pace'],
])('shows the reference for %s percent usage', async (used, expected) => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          limits: [
            {
              ...snapshot.limits[0],
              windows: [
                {
                  kind: 'primary',
                  used_percent: used,
                  window_seconds: 3600,
                  reset_at: now + 1800,
                },
              ],
            },
          ],
        }),
      ),
    ),
  )
  mount()
  expect(await screen.findByText(expected)).toBeTruthy()
})

it.each([
  ['stale data', { stale: true }, {}],
  ['expired data', { expires_at: now }, {}],
  ['future observation', { updated_at: now + 1 }, {}],
  ['passed reset', {}, { reset_at: now }],
  ['unknown duration', {}, { kind: 'model', window_seconds: null }],
  ['unknown usage', {}, { used_percent: null }],
  ['out-of-range usage', {}, { used_percent: 101 }],
] satisfies [string, Partial<typeof snapshot>, Partial<UsageWindow>][])(
  'keeps pace unavailable for %s',
  async (_reason, invalidSnapshot, invalidWindow) => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            ...snapshot,
            ...invalidSnapshot,
            limits: [
              {
                ...snapshot.limits[0],
                windows: [
                  { ...snapshot.limits[0].windows[0], ...invalidWindow },
                ],
              },
            ],
          }),
        ),
      ),
    )
    mount()
    await screen.findByText(/^Updated /)
    expect(screen.queryByText(/even pace/)).toBeNull()
  },
)

it.each(['expiry', 'reset'] as const)(
  'removes the pace reference when the browser clock reaches %s',
  async (boundary) => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(now * 1000)
    try {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              ...snapshot,
              expires_at: now + (boundary === 'expiry' ? 30 : 120),
              limits: [
                {
                  ...snapshot.limits[0],
                  windows: [
                    {
                      kind: 'primary',
                      used_percent: 70,
                      window_seconds: 3600,
                      reset_at: now + (boundary === 'reset' ? 30 : 1800),
                    },
                  ],
                },
              ],
            }),
          ),
        ),
      )
      mount()
      await screen.findByText(/percentage points .* even pace/)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000)
      })
      expect(screen.queryByText(/even pace/)).toBeNull()
      expect(screen.getByText('30% remaining')).toBeTruthy()
    } finally {
      vi.useRealTimers()
    }
  },
)

it.each(['claude', 'antigravity'] as const)(
  'shows %s usage without Codex reset cards',
  async (provider) => {
    const limits =
      provider === 'claude'
        ? snapshot.limits
        : [
            {
              name: 'synthetic-model',
              allowed: null,
              limit_reached: null,
              windows: [
                {
                  kind: 'model',
                  used_percent: 25,
                  window_seconds: null,
                  reset_at: now + 7200,
                },
              ],
            },
            {
              name: 'synthetic-unknown',
              allowed: null,
              limit_reached: null,
              windows: [
                {
                  kind: 'model',
                  used_percent: null,
                  window_seconds: null,
                  reset_at: null,
                },
              ],
            },
          ]
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ ...snapshot, limits })),
        ),
    )
    mount(provider)
    await screen.findByText('75% remaining')
    expect(screen.queryByText(/Reset cards:/)).toBeNull()
    if (provider === 'antigravity') {
      expect(screen.getByText('synthetic-model')).toBeTruthy()
      expect(
        screen.getByRole('progressbar', { name: 'synthetic-model' }),
      ).toBeTruthy()
      expect(screen.getByText('Usage unavailable')).toBeTruthy()
      expect(screen.queryByText('100% remaining')).toBeNull()
    }
  },
)

it('keeps many model quotas compact and makes constrained models visible before expansion', async () => {
  const limits = Array.from({ length: 8 }, (_, index) => ({
    name: `synthetic-model-${index}`,
    allowed: null,
    limit_reached: null,
    windows: [
      {
        kind: 'model',
        used_percent: index === 7 ? 100 : index * 10,
        window_seconds: null,
        reset_at: now + 7200,
      },
    ],
  }))
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ ...snapshot, limits }))),
  )
  mount('antigravity')
  await screen.findByText('0% remaining')
  expect(screen.getAllByRole('progressbar')).toHaveLength(3)
  expect(
    screen.getByRole('progressbar', { name: 'synthetic-model-7' }),
  ).toBeTruthy()
  const user = userEvent.setup()
  const expand = screen.getByRole('button', { name: 'Show all 8 models' })
  expect(expand.getAttribute('aria-expanded')).toBe('false')
  await user.click(expand)
  expect(screen.getAllByRole('progressbar')).toHaveLength(8)
  const collapse = screen.getByRole('button', { name: 'Show fewer models' })
  expect(collapse.getAttribute('aria-expanded')).toBe('true')
  await user.click(collapse)
  expect(screen.getAllByRole('progressbar')).toHaveLength(3)
})

it('loads actual quota windows and refreshes without confusing failure with exhaustion', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(snapshot)))
  vi.stubGlobal('fetch', fetch)
  mount()
  await screen.findByText('75% remaining')
  expect(screen.getByText('35 percentage points behind even pace')).toBeTruthy()
  expect(screen.getByText('5-hour limit')).toBeTruthy()
  expect(screen.getByText('7-day limit')).toBeTruthy()
  expect(screen.getByText('0% remaining')).toBeTruthy()
  expect(screen.getAllByRole('progressbar')).toHaveLength(2)
  fetch.mockResolvedValueOnce(
    new Response('{"error":"upstream_unavailable"}', { status: 502 }),
  )
  await userEvent.setup().click(
    screen.getByRole('button', {
      name: 'Refresh usage for Test subscription',
    }),
  )
  await screen.findByText(
    'Refresh failed: upstream service is unavailable. Showing the last known usage.',
  )
  expect(screen.getByText('75% remaining')).toBeTruthy()
  expect(screen.queryByText(/even pace/)).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch.mock.calls[1][0]).toBe(
    '/api/accounts/synthetic-account/usage/refresh',
  )
  expect(fetch.mock.calls[1][1].method).toBe('POST')
})
it('shows reset cards separately and keeps missing counts unknown', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ ...snapshot, reset_credits: 2 })),
    )
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ ...snapshot, reset_credits: 0 })),
    )
  vi.stubGlobal('fetch', fetch)
  mount()
  await screen.findByText('Reset cards: 2')
  await userEvent.setup().click(
    screen.getByRole('button', {
      name: 'Refresh usage for Test subscription',
    }),
  )
  await screen.findByText('Reset cards: 0')
  expect(screen.queryByText('Reset cards: 2')).toBeNull()
})
it('shows an unknown reset card count when the provider omits it', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response(JSON.stringify(snapshot))),
  )
  mount()
  await screen.findByText('Reset cards: Unknown')
})
it('does not invent quota when no window or percentage is available', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({ ...metadata, updated_at: now, limits: [] }),
      ),
    )
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          ...metadata,
          updated_at: now,
          limits: [
            {
              name: '',
              allowed: null,
              limit_reached: null,
              windows: [
                {
                  kind: 'primary',
                  window_seconds: 3600,
                  used_percent: null,
                  reset_at: null,
                },
              ],
            },
          ],
        }),
      ),
    )
  vi.stubGlobal('fetch', fetch)
  mount()
  await screen.findByText('No usage limits reported')
  await userEvent.setup().click(
    screen.getByRole('button', {
      name: 'Refresh usage for Test subscription',
    }),
  )
  await screen.findByText('Usage unavailable')
  expect(screen.queryByText('100% remaining')).toBeNull()
  expect(screen.queryByRole('progressbar')).toBeNull()
  expect(screen.getByText('Reset time unavailable')).toBeTruthy()
})
it('shows a recoverable error when the initial request fails', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response('{"error":"upstream_rate_limited"}', { status: 429 }),
      ),
  )
  mount()
  await screen.findByText(
    'Unable to load usage: upstream rate limited. Try refreshing later.',
  )
  expect(screen.queryByRole('progressbar')).toBeNull()
})
it('does not infer renewed quota after a reset time passes and respects upstream restrictions', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          limits: [
            {
              ...snapshot.limits[0],
              allowed: false,
              windows: [
                { ...snapshot.limits[0].windows[0], reset_at: now - 1 },
              ],
            },
          ],
        }),
      ),
    ),
  )
  mount()
  await screen.findByText('Currently limited')
  await waitFor(() =>
    expect(screen.getByText('Reset time passed · refresh usage')).toBeTruthy(),
  )
  expect(screen.queryByText('100% remaining')).toBeNull()
})

it('counts down from the snapshot time without rounding a fresh four-day reset to five days', async () => {
  const future = now + 120
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...metadata,
          server_time: future,
          expires_at: future + 120,
          updated_at: future,
          limits: [
            {
              ...snapshot.limits[0],
              windows: [
                { ...snapshot.limits[0].windows[0], reset_at: future + 8340 },
                { ...snapshot.limits[0].windows[1], reset_at: future + 345600 },
              ],
            },
          ],
        }),
      ),
    ),
  )
  mount()
  await screen.findByText('Resets in 2h 19m')
  expect(screen.getByText('Resets in 4d 0h')).toBeTruthy()
})

it('marks a restored snapshot as stale while a shared refresh runs, then replaces it', async () => {
  const old = {
    ...snapshot,
    updated_at: now - 600,
    server_time: now,
    expires_at: now - 480,
    stale: true,
    refreshing: true,
    refresh_failed: false,
    retry_after_seconds: 0,
  }
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(old)))
    .mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            ...snapshot,
            server_time: now,
            expires_at: now + 120,
            stale: false,
            refreshing: false,
            refresh_failed: false,
            retry_after_seconds: 0,
          }),
        ),
      ),
    )
  vi.stubGlobal('fetch', fetch)
  mount()
  await screen.findByText('Cached usage is outdated. Refreshing…')
  expect(screen.getByText('75% remaining')).toBeTruthy()
  // A restored snapshot must use the response clock, not restart its countdown from the old observation.
  expect(screen.getByText('Resets in 2h 0m')).toBeTruthy()
  await waitFor(
    () =>
      expect(
        screen.queryByText('Cached usage is outdated. Refreshing…'),
      ).toBeNull(),
    { timeout: 2500 },
  )
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('shows server-side refresh failure with the persisted snapshot', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          server_time: now,
          expires_at: now + 120,
          stale: true,
          refreshing: false,
          refresh_failed: true,
          refresh_error: 'upstream_unavailable',
          retry_after_seconds: 30,
        }),
      ),
    ),
  )
  mount()
  await screen.findByText(
    'Refresh failed: upstream service is unavailable. Showing the last known usage.',
  )
  expect(screen.getByText('75% remaining')).toBeTruthy()
  expect(
    screen
      .getByRole('button', { name: 'Refresh usage for Test subscription' })
      .hasAttribute('disabled'),
  ).toBe(true)
})

it('explains a rate-limited refresh without exposing upstream details', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...snapshot,
          server_time: now,
          expires_at: now + 120,
          stale: true,
          refreshing: false,
          refresh_failed: true,
          refresh_error: 'rate_limited',
          retry_after_seconds: 30,
        }),
      ),
    ),
  )
  mount()
  await screen.findByText(
    'Refresh failed: upstream rate limited. Showing the last known usage.',
  )
  expect(screen.queryByText(/private|token|http/)).toBeNull()
})

it('allows retry if the connection is lost while waiting for a background refresh', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ ...snapshot, stale: true, refreshing: true }),
        ),
      )
      .mockImplementation(() =>
        Promise.resolve(
          new Response('{"error":"unavailable"}', { status: 503 }),
        ),
      ),
  )
  mount()
  await screen.findByText('Cached usage is outdated. Refreshing…')
  await screen.findByText(
    'Refresh failed: upstream service is unavailable. Showing the last known usage.',
    {},
    { timeout: 2500 },
  )
  expect(
    screen
      .getByRole('button', { name: 'Refresh usage for Test subscription' })
      .hasAttribute('disabled'),
  ).toBe(false)
})
