import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SchemeForm } from './SchemeForm'
import { parseAllocationValue } from '@/lib/allocations'

it('converts the three units without losing small values', () => {
  expect(parseAllocationValue('50.25', 'ratio')).toBe(5025)
  expect(parseAllocationValue('1.000001', 'tokens')).toBe(1000001)
  expect(parseAllocationValue('0.000001', 'amount')).toBe(1)
  expect(parseAllocationValue('0.001', 'ratio')).toBeNull()
  expect(parseAllocationValue('-1', 'amount')).toBeNull()
})
it('submits exactly one mode and rejects an overallocated ratio', async () => {
  const submit = vi.fn()
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SchemeForm
        teams={[
          {
            id: 1,
            name: 'Synthetic team',
            enabled: true,
            member_ids: [2],
            members: [{ id: 2, username: 'synthetic-member', enabled: true }],
            created_at: 1,
          },
        ]}
        groups={[{ id: 1, name: 'Synthetic pool', enabled: true }]}
        onSubmit={submit}
        onCancel={() => {}}
        pending={false}
      />
    </QueryClientProvider>,
  )
  await user.type(screen.getByLabelText('Scheme name'), 'Synthetic scheme')
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(
    screen.getByLabelText('Allowance for synthetic-member'),
    '1.5',
  )
  await user.click(
    screen.getByRole('button', { name: 'Save allocation scheme' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'tokens',
        period: 'month',
        members: [{ user_id: 2, limit: 1500000 }],
        rates: [],
      },
    }),
  )
  submit.mockClear()
  await user.click(screen.getByLabelText('By share'))
  const allowance = screen.getByLabelText('Allowance for synthetic-member')
  await user.clear(allowance)
  await user.type(allowance, '101')
  await user.click(
    screen.getByRole('button', { name: 'Save allocation scheme' }),
  )
  expect(submit).not.toHaveBeenCalled()
  expect(screen.getByRole('alert')).toBeTruthy()
})

it('offers only enabled, nonempty, dedicated pools without a scheme', async () => {
  const { availableAllocationPools } = await import('@/lib/allocations')
  const pool = (id: number, enabled = true, account_count = 1) => ({
    id,
    enabled,
    account_count,
  })
  expect(
    availableAllocationPools(
      [pool(1), pool(2), pool(3, false), pool(4, true, 0), pool(5)],
      [{ group_id: 2 }],
    ),
  ).toEqual([pool(5)])
})

it('does not label a paused scheme balance as active', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic',
        team_id: 1,
        team_name: 'Synthetic',
        group_id: 2,
        group_name: 'Synthetic',
        enabled: false,
        effective_at: 1,
        created_at: 1,
        next: null,
        config: {
          mode: 'tokens',
          period: 'month',
          members: [{ user_id: 2, limit: 100 }],
          rates: [],
        },
        available: false,
        unassigned: 0,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            mode: 'tokens',
            limit: 100,
            used: 0,
            tokens: 0,
            pending: 0,
            reset_at: 2000000000,
            window_id: 0,
            window_kind: '',
          },
        ],
      }}
    />,
  )
  expect(screen.queryByText('Active')).toBeNull()
  expect(screen.getByText('Unavailable')).toBeTruthy()
})

it('changes team, pool and period with accessible dropdowns and clears old shares', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  const team = (id: number, name: string) => ({
    id,
    name,
    enabled: true,
    member_ids: [2],
    members: [{ id: 2, username: 'synthetic-member', enabled: true }],
    created_at: 1,
  })
  render(
    <SchemeForm
      teams={[
        team(1, 'Synthetic first team'),
        team(2, 'Synthetic second team'),
      ]}
      groups={[
        { id: 2, name: 'Synthetic first pool', enabled: true },
        { id: 3, name: 'Synthetic second pool', enabled: true },
      ]}
      onSubmit={submit}
      onCancel={() => {}}
      pending={false}
    />,
  )
  await user.type(screen.getByLabelText('Scheme name'), 'Synthetic')
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(
    screen.getByLabelText('Allowance for synthetic-member'),
    '1.5',
  )
  await user.click(screen.getByRole('button', { name: 'Personnel team' }))
  await user.keyboard('{End}{Enter}')
  expect(
    screen.getByRole('button', { name: 'Personnel team' }).textContent,
  ).toContain('Synthetic second team')
  expect(
    (
      screen.getByLabelText(
        'Allowance for synthetic-member',
      ) as HTMLInputElement
    ).value,
  ).toBe('')
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '2')
  await user.click(screen.getByRole('button', { name: 'Account group' }))
  await user.click(
    screen.getByRole('menuitemradio', {
      name: 'Synthetic second pool',
    }),
  )
  await user.click(screen.getByRole('button', { name: 'Reset period' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'Daily · UTC' }))
  await user.click(
    screen.getByRole('button', { name: 'Save allocation scheme' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      team_id: 2,
      group_id: 3,
      config: {
        mode: 'tokens',
        period: 'day',
        members: [{ user_id: 2, limit: 2000000 }],
        rates: [],
      },
    }),
  )
})
