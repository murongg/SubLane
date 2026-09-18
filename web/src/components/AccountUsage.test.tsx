import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { AccountUsage } from './AccountUsage'

const now = Math.floor(Date.now() / 1000)
const snapshot = {
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
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccountUsage id="synthetic-account" name="Test subscription" />
    </QueryClientProvider>,
  )
  return client
}
it('loads actual quota windows and refreshes without confusing failure with exhaustion', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(snapshot)))
  vi.stubGlobal('fetch', fetch)
  mount()
  await screen.findByText('75% remaining')
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
  await screen.findByText('Unable to refresh. Showing the last known usage.')
  expect(screen.getByText('75% remaining')).toBeTruthy()
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('does not invent quota when no window or percentage is available', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ updated_at: now, limits: [] })),
    )
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
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
  await screen.findByText('Unable to load usage. Try refreshing later.')
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
