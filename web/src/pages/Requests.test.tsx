import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated, memberAuthenticated } from '@/test/fixtures'

it('shows request metadata and filters failed calls', async () => {
  const fetch = vi.fn().mockImplementation((url: string) => {
    let data: unknown = { accounts: [] }
    if (url === '/api/auth/state') data = authenticated
    if (url.startsWith('/api/requests?'))
      data = {
        requests: [
          {
            id: 1,
            user_id: 2,
            key_id: 1,
            group_id: 1,
            account_id: 'synthetic-account',
            provider: 'codex',
            model: 'codex/synthetic-model',
            transport: 'http',
            operation: 'responses',
            started_at: 1900000000,
            duration_ms: 125,
            outcome: 'error',
            error_code: 'rate_limited',
            upstream_status: 429,
            input_tokens: null,
            output_tokens: null,
            cached_tokens: null,
            username: 'member-test',
            key_name: 'Synthetic client',
            group_name: 'Default',
            account_name: 'Synthetic account',
          },
        ],
        next_cursor: 0,
      }
    return Promise.resolve(new Response(JSON.stringify(data)))
  })
  vi.stubGlobal('fetch', fetch)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/admin/requests'] }),
      )}
    />,
  )
  await screen.findByRole('heading', { name: 'All requests' })
  await screen.findByText('Synthetic account')
  expect(screen.getByText(/^Rate limited/)).toBeTruthy()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Result filter' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'Failed' }))
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(([url]) => url.includes('outcome=error')),
    ).toBe(true),
  )
})

it.each([memberAuthenticated, authenticated])(
  'shows only the personal history endpoint for $user.role',
  async (session) => {
    const fetch = vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(new Response(JSON.stringify(session)))
      if (url.startsWith('/api/me/requests?'))
        return Promise.resolve(
          new Response(
            JSON.stringify({
              requests: [
                {
                  id: 1,
                  user_id: session.user.id,
                  key_id: 1,
                  group_id: 1,
                  account_id: '',
                  account_name: '',
                  provider: 'codex',
                  model: 'codex/synthetic-model',
                  transport: 'http',
                  operation: 'responses',
                  started_at: 1900000000,
                  duration_ms: 125,
                  outcome: 'success',
                  error_code: '',
                  upstream_status: 200,
                  input_tokens: 12,
                  output_tokens: 4,
                  cached_tokens: null,
                  username: session.user.username,
                  key_name: 'My synthetic client',
                  group_name: 'Default',
                },
              ],
              next_cursor: 0,
            }),
          ),
        )
      return Promise.reject(new Error('Unexpected management request'))
    })
    vi.stubGlobal('fetch', fetch)
    render(
      <App
        router={createAppRouter(
          createMemoryHistory({ initialEntries: ['/requests'] }),
        )}
      />,
    )
    await screen.findByRole('heading', { name: 'Your requests' })
    await screen.findByText('My synthetic client')
    expect(screen.queryByRole('button', { name: 'Account filter' })).toBeNull()
    expect(screen.getByRole('columnheader', { name: 'API key' })).toBeTruthy()
    expect(screen.getByRole('columnheader', { name: 'Group' })).toBeTruthy()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Result filter' }))
    await user.click(screen.getByRole('menuitemradio', { name: 'Failed' }))
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(
          ([url]) =>
            url.startsWith('/api/me/requests?') &&
            url.includes('outcome=error'),
        ),
      ).toBe(true),
    )
    expect(
      fetch.mock.calls.every(
        ([url]) =>
          url === '/api/auth/state' || url.startsWith('/api/me/requests?'),
      ),
    ).toBe(true)
  },
)
