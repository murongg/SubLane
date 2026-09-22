import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Teams } from './Teams'

vi.mock('@/lib/allocations', async () => {
  const actual = await vi.importActual('@/lib/allocations')
  return {
    ...actual,
    teamsOptions: {
      queryKey: ['teams'],
      queryFn: () =>
        Promise.resolve({
          teams: [
            {
              id: 1,
              name: 'Synthetic team',
              enabled: true,
              member_ids: [2],
              group_ids: [],
              members: [{ id: 2, username: 'synthetic-member', enabled: true }],
              created_at: 1,
            },
          ],
        }),
    },
    schemesOptions: {
      queryKey: ['allocations'],
      queryFn: () => Promise.resolve({ schemes: [] }),
    },
  }
})
vi.mock('@/lib/groups', () => ({
  groupOptions: {
    queryKey: ['groups'],
    queryFn: () =>
      Promise.resolve({
        groups: [
          { id: 2, name: 'Synthetic pool', enabled: true, account_count: 1 },
        ],
      }),
  },
}))
vi.mock('@/lib/members', () => ({
  getMembers: () => Promise.resolve({ members: [], next_cursor: 0 }),
}))

it('shows team resources beside team membership settings', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Teams />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Synthetic team')).toBeTruthy()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Edit Synthetic team' }))
  expect(screen.getByText('Team resources')).toBeTruthy()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Add team resource' }))
  expect(screen.getByText('Synthetic team')).toBeTruthy()
  expect(screen.queryByLabelText('Scheme name')).toBeNull()
})
