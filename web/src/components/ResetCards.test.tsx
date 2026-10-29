import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { ResetCards } from './ResetCards'

const inventory = {
  available_count: 2,
  observed_at: 1900000000,
  next_card_id: 'synthetic-soon',
  cards: [
    {
      id: 'synthetic-soon',
      title: 'Synthetic soon',
      status: 'available',
      supported: true,
      granted_at: 1800000000,
      expires_at: 2000000000,
    },
    {
      id: 'synthetic-bank',
      title: 'Synthetic bank',
      status: 'available',
      supported: true,
      granted_at: 1800000000,
      expires_at: null,
    },
    {
      id: 'synthetic-used',
      title: 'Synthetic used',
      status: 'redeemed',
      supported: true,
      granted_at: 1800000000,
      expires_at: null,
    },
  ],
}
function mount(pending = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ResetCards
        id="synthetic-account"
        enabled
        timeZone="UTC"
        pending={pending}
      />
    </QueryClientProvider>,
  )
  return client
}

it('shows expiry priority, states and non-expiring cards without rendering opaque IDs', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(inventory))),
  )
  mount()
  await screen.findByText('Synthetic soon')
  const rows = screen.getAllByRole('listitem')
  expect(within(rows[0]).getByText('Next to use')).toBeTruthy()
  expect(within(rows[1]).getByText('No expiry')).toBeTruthy()
  expect(within(rows[2]).getByText('Redeemed')).toBeTruthy()
  expect(screen.queryByText('synthetic-soon')).toBeNull()
})
it('retains the last list with a warning when refreshing fails', async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(inventory)))
    .mockResolvedValue(
      new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 }),
    )
  vi.stubGlobal('fetch', fetcher)
  mount()
  await screen.findByText('Synthetic soon')
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Refresh cards' }))
  await screen.findByText('Could not refresh reset cards. Try again.')
  expect(screen.getByText('Synthetic soon')).toBeTruthy()
})
it('offers retry on an initial failure and distinguishes an empty inventory', async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 }),
    )
    .mockResolvedValue(
      new Response(
        JSON.stringify({
          ...inventory,
          cards: [],
          available_count: 0,
          next_card_id: '',
        }),
      ),
    )
  vi.stubGlobal('fetch', fetcher)
  mount()
  await screen.findByText('Could not refresh reset cards. Try again.')
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Refresh cards' }))
  await screen.findByText('No reset card details reported.')
})
it('does not suggest a different card while a prior attempt is pending', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(inventory))),
  )
  mount(true)
  await screen.findByText('Synthetic soon')
  expect(screen.queryByText('Next to use')).toBeNull()
})
it('does not read disabled account credentials', async () => {
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <ResetCards
        id="synthetic-account"
        enabled={false}
        timeZone="UTC"
        pending={false}
      />
    </QueryClientProvider>,
  )
  await waitFor(() => expect(fetcher).not.toHaveBeenCalled())
})
