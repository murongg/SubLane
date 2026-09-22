import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import {
  authenticated,
  memberAuthenticated,
  hourlyActivity,
} from '@/test/fixtures'

const metrics = {
  requests: 2,
  completed: 1,
  incomplete: 0,
  errors: 1,
  canceled: 0,
  rejected: 0,
  duration_ms: 840,
  input_tokens: 120,
  output_tokens: 38,
  cached_tokens: 0,
  input_reported: 1,
  output_reported: 1,
  cached_reported: 0,
}
const summary = {
  activity: hourlyActivity,
  from_day: 1900000000,
  to_day: 1900086400,
  tracking_since: 1900000000,
  totals: metrics,
  days: [{ id: '1900000000', name: '2030-03-17', ...metrics }],
  members: [],
  models: [
    { id: 'codex/synthetic-model', name: 'codex/synthetic-model', ...metrics },
  ],
  groups: [],
}

it.each([
  ['/usage', memberAuthenticated, '/api/me/usage'],
  ['/admin/usage', authenticated, '/api/usage'],
] as const)(
  'shows honest usage totals at %s and changes the period',
  async (path, session, endpoint) => {
    const fetch = vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(new Response(JSON.stringify(session)))
      if (url === '/api/me/allocations')
        return Promise.resolve(new Response(JSON.stringify({ schemes: [] })))
      if (url === '/api/me/budgets')
        return Promise.resolve(
          new Response(JSON.stringify({ rules: [], pending: [] })),
        )
      if (url === '/api/me/limits')
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user_id: 2,
              requests_per_minute: 0,
              max_concurrency: 0,
              in_flight: 0,
              requests_this_minute: 0,
              reset_at: 1900000060,
            }),
          ),
        )
      if (url.startsWith(endpoint + '?'))
        return Promise.resolve(new Response(JSON.stringify(summary)))
      return Promise.reject(new Error('Unexpected privileged data request'))
    })
    vi.stubGlobal('fetch', fetch)
    const user = userEvent.setup()
    render(
      <App
        router={createAppRouter(
          createMemoryHistory({ initialEntries: [path] }),
        )}
      />,
    )
    await screen.findByRole('heading', {
      name: path === '/usage' ? 'Your usage' : 'Team usage',
    })
    await screen.findByText(
      'Some calls did not report token usage; totals include reported values only.',
    )
    await screen.findByRole('img', { name: 'Daily · Requests' })
    await screen.findByRole('region', { name: 'Activity by time' })
    expect(screen.getAllByRole('gridcell')).toHaveLength(168)
    if (path === '/usage')
      await screen.findByText(
        'No token budgets for keys outside allocation schemes.',
      )
    else
      expect(
        screen.queryByRole('heading', { name: 'Token budgets' }),
      ).toBeNull()
    const reads = fetch.mock.calls.filter(
      ([url]) => url === endpoint + '?days=7',
    ).length
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() =>
      expect(
        fetch.mock.calls.filter(([url]) => url === endpoint + '?days=7').length,
      ).toBeGreaterThan(reads),
    )
    expect(screen.queryByRole('table')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'View details' }))
    expect(screen.getByRole('table', { name: 'Daily' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Hide details' }))
    await user.click(screen.getByRole('button', { name: 'Usage period' }))
    await user.click(
      screen.getByRole('menuitemradio', { name: 'Last 30 days' }),
    )
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(([url]) => url === endpoint + '?days=30'),
      ).toBe(true),
    )
    await user.click(screen.getByRole('button', { name: 'Breakdown' }))
    await user.click(screen.getByRole('menuitemradio', { name: 'Models' }))
    await screen.findByText('codex/synthetic-model')
    expect(
      screen.getByRole('region', { name: 'Activity by time' }),
    ).toBeTruthy()
    expect(
      fetch.mock.calls.every(
        ([url]) =>
          url === '/api/auth/state' ||
          url === '/api/me/limits' ||
          url === '/api/me/budgets' ||
          url === '/api/me/allocations' ||
          url.startsWith(endpoint + '?'),
      ),
    ).toBe(true)
  },
)
