import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { authKey } from '@/lib/auth'
import { authenticated } from '@/test/fixtures'
import { ExportCSV } from './ExportCSV'

it('reports an export failure and allows retry without disabling the page', async () => {
  const client = new QueryClient()
  client.setQueryData(authKey, authenticated)
  const load = vi.fn().mockRejectedValue(new Error('synthetic failure'))
  render(
    <QueryClientProvider client={client}>
      <ExportCSV
        load={load}
        filename="synthetic.csv"
        userID={authenticated.user.id}
        disabled={false}
      />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'Could not export CSV',
  )
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect(load).toHaveBeenCalledTimes(2)
})
