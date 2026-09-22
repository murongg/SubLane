import { render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Teams } from './Teams'
import {
  saveTeam,
  schemesOptions,
  teamsOptions,
  type Scheme,
} from '@/lib/allocations'

vi.mock('@/lib/allocations', async () => {
  const actual = await vi.importActual('@/lib/allocations')
  return {
    ...actual,
    saveTeam: vi.fn().mockResolvedValue({}),
    teamsOptions: {
      queryKey: ['teams'],
      queryFn: vi.fn().mockResolvedValue({
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
      queryFn: vi
        .fn<() => Promise<{ schemes: Scheme[] }>>()
        .mockResolvedValue({ schemes: [] }),
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
  getMembers: () =>
    Promise.resolve({
      members: [{ id: 2, username: 'synthetic-member', enabled: true }],
      next_cursor: 0,
    }),
}))

function mount() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <Teams />
    </QueryClientProvider>,
  )
}
const limited: Scheme = {
  id: 1,
  name: 'Synthetic limit',
  team_id: 1,
  team_name: 'Synthetic team',
  group_id: 2,
  group_name: 'Synthetic pool',
  enabled: true,
  created_at: 1,
  effective_at: 1,
  next: null,
  config: {
    mode: 'tokens',
    period: 'month',
    members: [{ user_id: 2, limit: 1000000 }],
    rates: [],
  },
}
it('defaults to free pool access without asking for an allocation rule', async () => {
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [] })
  vi.mocked(saveTeam).mockClear()
  const user = userEvent.setup()
  mount()
  await user.click(
    await screen.findByRole('button', { name: 'Edit Synthetic team' }),
  )
  expect(screen.getByText('Free use · no team allowance')).toBeTruthy()
  expect(
    screen.queryByRole('button', { name: 'Add resource allowance' }),
  ).toBeNull()
  expect(screen.queryByRole('radio', { name: 'By share' })).toBeNull()
  await user.click(screen.getByRole('checkbox', { name: 'Synthetic pool' }))
  await user.click(screen.getByRole('button', { name: 'Save team' }))
  await waitFor(() =>
    expect(saveTeam).toHaveBeenCalledWith(
      {
        id: 1,
        name: 'Synthetic team',
        enabled: true,
        member_ids: [2],
        group_ids: [2],
      },
      expect.anything(),
    ),
  )
})
it('reveals the three allowance modes only after opting into advanced settings', async () => {
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [] })
  const user = userEvent.setup()
  mount()
  await user.click(
    await screen.findByRole('button', { name: 'Edit Synthetic team' }),
  )
  await user.click(screen.getByText('Advanced settings: usage limits'))
  await user.click(
    screen.getByRole('button', { name: 'Add resource allowance' }),
  )
  for (const name of ['By share', 'By amount', 'By tokens']) {
    expect(screen.getByRole('radio', { name })).toBeTruthy()
  }
  await user.click(screen.getByRole('radio', { name: 'By tokens' }))
  await user.type(
    screen.getByLabelText('Allowance for synthetic-member'),
    '2.5',
  )
  await user.click(screen.getByText('Advanced settings: usage limits'))
  await user.click(screen.getByText('Advanced settings: usage limits'))
  expect(
    (
      screen.getByLabelText(
        'Allowance for synthetic-member',
      ) as HTMLInputElement
    ).value,
  ).toBe('2.5')
})
it('keeps existing limits and prevents reserved pools being granted as free access', async () => {
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [limited] })
  vi.mocked(saveTeam).mockClear()
  const user = userEvent.setup()
  mount()
  await user.click(
    await screen.findByRole('button', { name: 'Edit Synthetic team' }),
  )
  expect(screen.getByText('1 configured usage limits')).toBeTruthy()
  expect(screen.queryByRole('checkbox', { name: 'Synthetic pool' })).toBeNull()
  await user.click(screen.getByText('Advanced settings: usage limits'))
  await user.click(
    screen.getByRole('button', { name: 'Edit resource allowance' }),
  )
  expect(
    (screen.getByRole('radio', { name: 'By tokens' }) as HTMLInputElement)
      .checked,
  ).toBe(true)
  expect(
    (
      screen.getByLabelText(
        'Allowance for synthetic-member',
      ) as HTMLInputElement
    ).value,
  ).toBe('1')
})
it('does not offer free access when existing usage limits could not be loaded', async () => {
  vi.mocked(schemesOptions.queryFn!).mockRejectedValue(
    new Error('synthetic unavailable'),
  )
  mount()
  expect(await screen.findByRole('alert')).toBeTruthy()
  expect(
    screen.queryByRole('button', { name: 'Edit Synthetic team' }),
  ).toBeNull()
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [] })
})

it('creates a team with free pool access without choosing a quota mode', async () => {
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [] })
  vi.mocked(saveTeam).mockClear()
  const user = userEvent.setup()
  mount()
  await screen.findByRole('button', { name: 'Edit Synthetic team' })
  await user.click(screen.getByRole('button', { name: 'Create team' }))
  await user.type(screen.getByLabelText('Team name'), 'Synthetic new team')
  await user.click(
    await screen.findByRole('checkbox', { name: 'synthetic-member' }),
  )
  await user.click(screen.getByRole('checkbox', { name: 'Synthetic pool' }))
  await user.click(screen.getByRole('button', { name: 'Save team' }))
  await waitFor(() =>
    expect(saveTeam).toHaveBeenCalledWith(
      {
        id: undefined,
        name: 'Synthetic new team',
        enabled: true,
        member_ids: [2],
        group_ids: [2],
      },
      expect.anything(),
    ),
  )
})
it('can save team membership after a previously shared pool becomes reserved', async () => {
  vi.mocked(teamsOptions.queryFn!).mockResolvedValue({
    teams: [
      {
        id: 1,
        name: 'Synthetic team',
        enabled: true,
        created_at: 1,
        member_ids: [2],
        group_ids: [2],
        members: [{ id: 2, username: 'synthetic-member', enabled: true }],
      },
    ],
  })
  vi.mocked(schemesOptions.queryFn!).mockResolvedValue({ schemes: [limited] })
  vi.mocked(saveTeam).mockClear()
  const user = userEvent.setup()
  mount()
  await user.click(
    await screen.findByRole('button', { name: 'Edit Synthetic team' }),
  )
  await user.click(screen.getByRole('button', { name: 'Save team' }))
  await waitFor(() =>
    expect(saveTeam).toHaveBeenCalledWith(
      {
        id: 1,
        name: 'Synthetic team',
        enabled: true,
        member_ids: [2],
        group_ids: [],
      },
      expect.anything(),
    ),
  )
})
