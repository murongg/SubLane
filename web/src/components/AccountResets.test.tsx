import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { AccountResets } from './AccountResets'
import { AccountUsage } from './AccountUsage'

const initial = {
  config: {
    scheduled: false,
    exhausted: false,
    frequency: 'daily',
    time: '09:00',
    date: '',
    weekday: 1,
    daily_limit: 1,
  },
  next_run_at: 0,
  time_zone: 'UTC',
  used_today: 0,
  pending: false,
  history: [],
}
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccountResets
        id="synthetic-account"
        name="Synthetic subscription"
        enabled
      />
    </QueryClientProvider>,
  )
}
it('saves both triggers and the shared daily limit', async () => {
  let saved: unknown
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, options?: RequestInit) => {
      if (options?.method === 'PUT') {
        saved = JSON.parse(String(options.body))
        return new Response(JSON.stringify({ ...initial, config: saved }))
      }
      return new Response(JSON.stringify(initial))
    }),
  )
  mount()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Reset card settings' }))
  await user.click(
    await screen.findByRole('checkbox', { name: 'Use at a scheduled time' }),
  )
  await user.click(
    screen.getByRole('checkbox', { name: 'Use when quota is exhausted' }),
  )
  const limit = screen.getByLabelText('Daily card limit')
  await user.clear(limit)
  await user.type(limit, '2')
  await user.click(screen.getByRole('button', { name: 'Save settings' }))
  await waitFor(() =>
    expect(saved).toEqual({
      ...initial.config,
      scheduled: true,
      exhausted: true,
      daily_limit: 2,
    }),
  )
})

it('offers only same-attempt recovery while an outcome is unknown', async () => {
  const fetcher = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          ...initial,
          pending: true,
          used_today: 1,
          history: [
            {
              trigger: 'manual',
              outcome: 'pending',
              reason: 'confirmation_required',
              started_at: 1900000000,
            },
          ],
        }),
      ),
  )
  vi.stubGlobal('fetch', fetcher)
  mount()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Reset card settings' }))
  const retry = await screen.findByRole('button', {
    name: 'Check pending result',
  })
  expect(screen.queryByRole('button', { name: 'Use one card now' })).toBeNull()
  await user.click(retry)
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining('/resets/retry'),
      expect.objectContaining({ method: 'POST' }),
    ),
  )
})

it('keeps settings unavailable after a failed read and offers retry', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 }),
    ),
  )
  mount()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Reset card settings' }))
  await screen.findByText('Could not load reset card settings. Try again.')
  expect(screen.queryByRole('button', { name: 'Save settings' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
})

it('refreshes the visible card count after a background redemption is observed', async () => {
  const now = Math.floor(Date.now() / 1000)
  let redeemed = false
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify(
            url.endsWith('/resets')
              ? {
                  ...initial,
                  used_today: redeemed ? 1 : 0,
                  history: redeemed
                    ? [
                        {
                          trigger: 'exhausted',
                          outcome: 'success',
                          reason: 'reset',
                          started_at: now,
                        },
                      ]
                    : [],
                }
              : {
                  reset_credits: redeemed ? 2 : 3,
                  limits: [],
                  updated_at: now,
                  server_time: now,
                  expires_at: now + 120,
                  stale: false,
                  refreshing: false,
                  refresh_failed: false,
                  retry_after_seconds: 0,
                },
          ),
        ),
    ),
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccountUsage
        id="synthetic-account"
        name="Synthetic subscription"
        provider="codex"
      />
      <AccountResets
        id="synthetic-account"
        name="Synthetic subscription"
        enabled
      />
    </QueryClientProvider>,
  )
  await screen.findByText('Reset cards: 3')
  redeemed = true
  await client.invalidateQueries({
    queryKey: ['accounts', 'resets', 'synthetic-account'],
  })
  await screen.findByText('Reset cards: 2')
})
