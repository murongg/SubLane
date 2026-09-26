import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { SchemeForm } from './SchemeForm'
import { parseAllocationValue } from '@/lib/allocations'

it('assigns a pool allowance directly to its granted members', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  render(
    <SchemeForm
      groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
      groupID={2}
      onGroupChange={() => {}}
      members={[{ id: 7, username: 'synthetic-member' }]}
      onSubmit={submit}
      onCancel={() => {}}
      pending={false}
    />,
  )
  await user.type(
    screen.getByLabelText('Resource allowance name'),
    'Pool allowance',
  )
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '1')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      group_id: 2,
      config: expect.objectContaining({
        members: [{ user_id: 7, limit: 1000000 }],
      }),
    }),
  )
})

it('requires a total token budget and saves percentage shares', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  render(
    <SchemeForm
      groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
      groupID={2}
      onGroupChange={() => {}}
      members={[{ id: 7, username: 'synthetic-member' }]}
      onSubmit={submit}
      onCancel={() => {}}
      pending={false}
    />,
  )
  await user.type(
    screen.getByLabelText('Resource allowance name'),
    'Synthetic shares',
  )
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '25')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).not.toHaveBeenCalled()
  await user.type(screen.getByLabelText('Total budget'), '1')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'ratio',
        period: 'month',
        reset_day: 1,
        reset_time: '00:00',
        ratio_unit: 'tokens',
        total: 1_000_000,
        members: [{ user_id: 7, limit: 2500 }],
        rates: [],
      },
    }),
  )
})

it('saves separate per-member five-hour and seven-day amount limits', async () => {
  const user = userEvent.setup()
  const submit = mountModelPrices('synthetic-basic')
  await user.click(screen.getByLabelText('5-hour and 7-day limits'))
  await user.type(
    screen.getByLabelText('5-hour limit for synthetic-member'),
    '1',
  )
  await user.type(
    screen.getByLabelText('7-day limit for synthetic-member'),
    '10',
  )
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'windows',
        period: 'dual',
        members: [{ user_id: 2, limit: 1_000_000, limit_7d: 10_000_000 }],
        rates: [
          {
            model: 'synthetic-basic',
            input: 3_000_000,
            cached: 0,
            output: 7_000_000,
          },
        ],
      },
    }),
  )
})

it('offers an internal USD basis below the share choice', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  render(
    <SchemeForm
      groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
      groupID={2}
      onGroupChange={() => {}}
      members={[{ id: 7, username: 'synthetic-member' }]}
      onSubmit={submit}
      onCancel={() => {}}
      pending={false}
    />,
  )
  await user.type(
    screen.getByLabelText('Resource allowance name'),
    'Synthetic shares',
  )
  await user.click(screen.getByLabelText('By amount share'))
  await user.type(screen.getByLabelText('Total budget'), '2')
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '25')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'ratio',
        ratio_unit: 'amount',
        total: 2_000_000,
        period: 'month',
        reset_day: 1,
        reset_time: '00:00',
        members: [{ user_id: 7, limit: 2500 }],
        rates: [],
      },
    }),
  )
})

it('splits amount shares exactly and preserves saved prices while editing', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 1, username: 'synthetic-admin', role: 'admin' },
  })
  const team = {
    id: 1,
    name: 'Synthetic team',
    enabled: true,
    created_at: 1,
    member_ids: [2, 3, 4],
    members: [2, 3, 4].map((id) => ({
      id,
      username: `synthetic-${id}`,
      enabled: true,
    })),
  }
  const rates = [
    {
      model: 'synthetic-model',
      input: 1000000,
      cached: 100000,
      output: 4000000,
    },
  ]
  render(
    <QueryClientProvider client={client}>
      <SchemeForm
        groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
        groupID={2}
        onGroupChange={() => {}}
        members={team.members}
        scheme={{
          id: 1,
          name: 'Synthetic',
          group_id: 2,
          group_name: 'Synthetic pool',
          enabled: true,
          created_at: 1,
          effective_at: 1,
          next: null,
          config: {
            mode: 'ratio',
            ratio_unit: 'amount',
            total: 1_000_000,
            period: 'month',
            members: [],
            rates,
          },
        }}
        pending={false}
        onCancel={() => {}}
        onSubmit={submit}
      />
    </QueryClientProvider>,
  )
  expect(
    screen.getByText('Metering and activation settings').closest('details')
      ?.open,
  ).toBe(true)
  expect(screen.getByLabelText('Model ID')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Split equally' }))
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'ratio',
        ratio_unit: 'amount',
        total: 1_000_000,
        period: 'month',
        reset_day: 1,
        reset_time: '00:00',
        members: [
          { user_id: 2, limit: 3334 },
          { user_id: 3, limit: 3333 },
          { user_id: 4, limit: 3333 },
        ],
        rates,
      },
    }),
  )
})

it('explains token-share balances without upstream quota settlement', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic shares',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'ratio',
          period: 'month',
          ratio_unit: 'tokens',
          total: 1_000_000,
          members: [{ user_id: 2, limit: 2500 }],
          rates: [],
        },
        available: true,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: 'month',
            mode: 'tokens',
            limit: 250_000,
            used: 210_000,
            tokens: 210_000,
            pending: 1,
            pending_current: 1,
            in_flight: 0,
            reserved: 25_000,
            admission_room: 40_000,
            admission: 'active',
            reset_at: 2_000_000_000,
          },
        ],
      }}
    />,
  )
  expect(
    screen.getByText(/total token budget × assigned percentage/),
  ).toBeTruthy()
  expect(screen.getByText('0.25 M')).toBeTruthy()
  expect(screen.getByText('Active')).toBeTruthy()
  expect(screen.getByText(/0 in flight · 1 awaiting usage/)).toBeTruthy()
})

it('shows a risk pause and provisional usage beside positive remaining allowance', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic shares',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'ratio',
          period: 'month',
          ratio_unit: 'tokens',
          total: 1_000_000,
          members: [{ user_id: 2, limit: 2500 }],
          rates: [],
        },
        available: true,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: 'month',
            mode: 'tokens',
            limit: 250_000,
            used: 240_000,
            tokens: 240_000,
            pending: 2,
            pending_current: 1,
            in_flight: 1,
            reserved: 50_000,
            admission_room: 0,
            admission: 'risk_limited',
            reset_at: 2_000_000_000,
          },
        ],
      }}
    />,
  )
  expect(screen.getByText('New requests temporarily paused')).toBeTruthy()
  expect(screen.getByText('0.01 M')).toBeTruthy()
  expect(screen.getByText(/1 in flight · 1 awaiting usage/)).toBeTruthy()
  expect(screen.getByText(/0.05 M temporarily reserved/)).toBeTruthy()
  expect(screen.getByText(/Admission headroom: 0 M/)).toBeTruthy()
  expect(screen.getByText(/Older pending requests: 1/)).toBeTruthy()
})

it('shows both windows under one member and the blocking seven-day status', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic dual',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'windows',
          period: 'dual',
          members: [{ user_id: 2, limit: 250_000, limit_7d: 2_500_000 }],
          rates: [
            {
              model: 'synthetic-model',
              input: 1_000_000,
              cached: 0,
              output: 1_000_000,
            },
          ],
        },
        available: true,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: '5h',
            mode: 'amount',
            limit: 250_000,
            used: 100_000,
            tokens: 100_000,
            pending: 0,
            pending_current: 0,
            in_flight: 0,
            reserved: 0,
            admission_room: 175_000,
            admission: 'active',
            reset_at: 2_000_000_000,
          },
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: '7d',
            mode: 'amount',
            limit: 2_500_000,
            used: 2_500_000,
            tokens: 2_500_000,
            pending: 0,
            pending_current: 0,
            in_flight: 0,
            reserved: 0,
            admission_room: 250_000,
            admission: 'exhausted',
            reset_at: 2_000_500_000,
          },
        ],
      }}
    />,
  )
  expect(
    screen.getAllByRole('heading', { name: 'synthetic-member' }),
  ).toHaveLength(1)
  expect(screen.getByText('Allowance exhausted')).toBeTruthy()
  expect(screen.getByText('5 hours')).toBeTruthy()
  expect(screen.getByText('7 days')).toBeTruthy()
  expect(screen.getByText('0.15 USD')).toBeTruthy()
  expect(screen.getByText('0 USD')).toBeTruthy()
})

it('keeps the current cycle active when only an older request is pending', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'tokens',
          period: 'day',
          members: [{ user_id: 2, limit: 100 }],
          rates: [],
        },
        available: true,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: 'day',
            mode: 'tokens',
            limit: 100,
            used: 0,
            tokens: 0,
            pending: 1,
            pending_current: 0,
            in_flight: 0,
            reserved: 0,
            admission_room: 110,
            admission: 'active',
            reset_at: 2_000_000_000,
          },
        ],
      }}
    />,
  )
  expect(screen.getByText('Active')).toBeTruthy()
  expect(screen.getByText(/Older pending requests: 1/)).toBeTruthy()
  expect(screen.queryByText('Usage pending reconciliation')).toBeNull()
})

it('shows exhausted amount-share balances even with pending usage', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic amount shares',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'ratio',
          ratio_unit: 'amount',
          total: 2_000_000,
          period: 'month',
          reset_day: 31,
          reset_time: '09:30',
          members: [{ user_id: 2, limit: 5000 }],
          rates: [
            {
              model: 'synthetic-model',
              input: 2_000_000,
              cached: 200_000,
              output: 4_000_000,
            },
          ],
        },
        available: true,
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: 'month',
            mode: 'amount',
            limit: 1_000_000,
            used: 1_000_000,
            tokens: 250_000,
            pending: 1,
            pending_current: 1,
            in_flight: 0,
            reserved: 100_000,
            admission_room: 0,
            admission: 'exhausted',
            reset_at: 2_000_000_000,
          },
        ],
      }}
    />,
  )
  expect(
    screen.getByText(/total internal USD budget × assigned percentage/),
  ).toBeTruthy()
  expect(screen.getAllByText('1 USD').length).toBeGreaterThan(0)
  expect(screen.getByText('Allowance exhausted')).toBeTruthy()
  expect(screen.getByText(/0\.25 M tokens/)).toBeTruthy()
  expect(screen.getByText(/Monthly on day 31 at 09:30 · UTC/)).toBeTruthy()
})

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
        groups={[{ id: 1, name: 'Synthetic pool', enabled: true }]}
        groupID={1}
        onGroupChange={() => {}}
        members={[{ id: 2, username: 'synthetic-member' }]}
        onSubmit={submit}
        onCancel={() => {}}
        pending={false}
      />
    </QueryClientProvider>,
  )
  await user.type(
    screen.getByLabelText('Resource allowance name'),
    'Synthetic scheme',
  )
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(
    screen.getByLabelText('Allowance for synthetic-member'),
    '1.5',
  )
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: {
        mode: 'tokens',
        period: 'month',
        reset_day: 1,
        reset_time: '00:00',
        members: [{ user_id: 2, limit: 1500000 }],
        rates: [],
      },
    }),
  )
  submit.mockClear()
  await user.click(screen.getByLabelText('By share'))
  await user.type(screen.getByLabelText('Total budget'), '1')
  const allowance = screen.getByLabelText('Allowance for synthetic-member')
  await user.clear(allowance)
  await user.type(allowance, '101')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).not.toHaveBeenCalled()
  expect(screen.getByRole('alert')).toBeTruthy()
})

it('keeps automatic amount-share prices collapsed while editing', () => {
  render(
    <SchemeForm
      groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
      groupID={2}
      onGroupChange={() => {}}
      members={[{ id: 2, username: 'synthetic-member' }]}
      scheme={{
        id: 1,
        name: 'Synthetic amount shares',
        group_id: 2,
        group_name: 'Synthetic pool',
        enabled: true,
        created_at: 1,
        effective_at: 1,
        next: null,
        config: {
          mode: 'ratio',
          ratio_unit: 'amount',
          total: 1_000_000,
          period: 'month',
          reset_day: 15,
          reset_time: '13:45',
          members: [],
          rates: [],
        },
      }}
      onSubmit={() => {}}
      onCancel={() => {}}
      pending={false}
    />,
  )
  expect(
    screen.getByRole('button', { name: 'Advanced: customize model prices' }),
  ).toBeTruthy()
  expect(screen.queryByLabelText('模型 ID')).toBeNull()
  expect(screen.getByLabelText('Day of month')).toHaveProperty('value', '15')
  expect(screen.getByLabelText('Reset time')).toHaveProperty('value', '13:45')
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
  ).toEqual([pool(1), pool(5)])
})

it('does not label a paused scheme balance as active', async () => {
  const { AllocationBalances } = await import('./AllocationBalances')
  render(
    <AllocationBalances
      detail={{
        id: 1,
        name: 'Synthetic',
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
        pending: [],
        balances: [
          {
            user_id: 2,
            username: 'synthetic-member',
            window_kind: 'month',
            mode: 'tokens',
            limit: 100,
            used: 0,
            tokens: 0,
            pending: 0,
            pending_current: 0,
            in_flight: 0,
            reserved: 0,
            admission_room: 110,
            admission: 'active',
            reset_at: 2000000000,
          },
        ],
      }}
    />,
  )
  expect(screen.queryByText('Active')).toBeNull()
  expect(screen.getByText('Unavailable')).toBeTruthy()
})

it('changes pool and period with accessible dropdowns and clears old shares', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  function Harness() {
    const [groupID, setGroupID] = useState(2)
    return (
      <SchemeForm
        groups={[
          { id: 2, name: 'Synthetic first pool', enabled: true },
          { id: 3, name: 'Synthetic second pool', enabled: true },
        ]}
        groupID={groupID}
        onGroupChange={setGroupID}
        members={[{ id: 2, username: 'synthetic-member' }]}
        onSubmit={submit}
        onCancel={() => {}}
        pending={false}
      />
    )
  }
  render(<Harness />)
  await user.type(screen.getByLabelText('Resource allowance name'), 'Synthetic')
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(
    screen.getByLabelText('Allowance for synthetic-member'),
    '1.5',
  )
  await user.click(screen.getByRole('button', { name: 'Account pool' }))
  await user.click(
    screen.getByRole('menuitemradio', { name: 'Synthetic second pool' }),
  )
  expect(
    (
      screen.getByLabelText(
        'Allowance for synthetic-member',
      ) as HTMLInputElement
    ).value,
  ).toBe('')
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '2')
  await user.click(screen.getByRole('button', { name: 'Reset period' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'Daily · UTC' }))
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      group_id: 3,
      config: {
        mode: 'tokens',
        period: 'day',
        reset_time: '00:00',
        members: [{ user_id: 2, limit: 2000000 }],
        rates: [],
      },
    }),
  )
})

it('saves a daily reset clock or monthly date and clock per allowance', async () => {
  const user = userEvent.setup()
  const submit = vi.fn()
  render(
    <SchemeForm
      groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
      groupID={2}
      onGroupChange={() => {}}
      members={[{ id: 7, username: 'synthetic-member' }]}
      onSubmit={submit}
      onCancel={() => {}}
      pending={false}
    />,
  )
  await user.type(screen.getByLabelText('Resource allowance name'), 'Synthetic')
  await user.click(screen.getByLabelText('By tokens'))
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '1')
  await user.clear(screen.getByLabelText('Day of month'))
  await user.type(screen.getByLabelText('Day of month'), '32')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).not.toHaveBeenCalled()
  await user.clear(screen.getByLabelText('Day of month'))
  await user.type(screen.getByLabelText('Day of month'), '31')
  fireEvent.change(screen.getByLabelText('Reset time'), {
    target: { value: '' },
  })
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Reset time'), {
    target: { value: '09:30' },
  })
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit.mock.calls[0][0].config).toEqual(
    expect.objectContaining({
      period: 'month',
      reset_day: 31,
      reset_time: '09:30',
    }),
  )
  await user.click(screen.getByRole('button', { name: 'Reset period' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'Daily · UTC' }))
  expect(screen.queryByLabelText('Day of month')).toBeNull()
  fireEvent.change(screen.getByLabelText('Reset time'), {
    target: { value: '18:45' },
  })
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit.mock.calls[1][0].config).toEqual(
    expect.objectContaining({ period: 'day', reset_time: '18:45' }),
  )
  expect(submit.mock.calls[1][0].config.reset_day).toBeUndefined()
})

const modelCatalog = {
  models: ['synthetic-basic', 'synthetic-pro'].map((id) => ({
    id,
    object: 'model',
    owned_by: 'synthetic',
  })),
  known_accounts: 1,
  unknown_accounts: 0,
  stale_accounts: 0,
  refreshing: false,
  refresh_failed: false,
  server_time: 1900000000,
}
function mountModelPrices(savedModel?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 1, username: 'synthetic-admin', role: 'admin' },
  })
  const submit = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <SchemeForm
        groups={[{ id: 2, name: 'Synthetic pool', enabled: true }]}
        groupID={2}
        onGroupChange={() => {}}
        members={[{ id: 2, username: 'synthetic-member' }]}
        scheme={
          savedModel
            ? {
                id: 1,
                name: 'Synthetic saved',
                group_id: 2,
                group_name: 'Synthetic pool',
                enabled: true,
                created_at: 1,
                effective_at: 1,
                next: null,
                config: {
                  mode: 'amount',
                  period: 'month',
                  members: [{ user_id: 2, limit: 10000000 }],
                  rates: [
                    {
                      model: savedModel,
                      input: 3000000,
                      cached: 0,
                      output: 7000000,
                    },
                  ],
                },
              }
            : undefined
        }
        pending={false}
        onCancel={() => {}}
        onSubmit={submit}
      />
    </QueryClientProvider>,
  )
  return submit
}
async function openModelPrices(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByLabelText('By amount'))
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  await waitFor(() =>
    expect(
      (screen.getByRole('button', { name: 'Model ID' }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  )
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
}
it('adds every missing pool model while preserving edited prices', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/models'))
        return new Response(JSON.stringify(modelCatalog))
      const models = new URL(url, 'http://example.test').searchParams.getAll(
        'model',
      )
      return new Response(
        JSON.stringify({
          prices: Object.fromEntries(
            models.map((model) => [
              model,
              {
                input: model === 'synthetic-pro' ? 5000000 : 1000000,
                cached: 100000,
                output: 8000000,
                source: 'synthetic',
              },
            ]),
          ),
        }),
      )
    }),
  )
  const user = userEvent.setup()
  const submit = mountModelPrices()
  await user.click(screen.getByLabelText('By amount'))
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  await user.click(
    await screen.findByRole('menuitemradio', { name: 'synthetic-basic' }),
  )
  await waitFor(() =>
    expect(
      (screen.getByLabelText('Input · USD / M') as HTMLInputElement).value,
    ).toBe('1'),
  )
  await user.clear(screen.getByLabelText('Input · USD / M'))
  await user.type(screen.getByLabelText('Input · USD / M'), '2')
  await user.click(
    screen.getByRole('button', { name: 'Add all available models (1)' }),
  )
  await waitFor(() =>
    expect(screen.getAllByRole('button', { name: 'Model ID' })).toHaveLength(2),
  )
  expect(
    (screen.getAllByLabelText('Input · USD / M')[0] as HTMLInputElement).value,
  ).toBe('2')
  await waitFor(() =>
    expect(
      (screen.getAllByLabelText('Input · USD / M')[1] as HTMLInputElement)
        .value,
    ).toBe('5'),
  )
  await user.type(screen.getByLabelText('Resource allowance name'), 'Synthetic')
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '10')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: expect.objectContaining({
        rates: [
          {
            model: 'synthetic-basic',
            input: 2000000,
            cached: 100000,
            output: 8000000,
          },
          {
            model: 'synthetic-pro',
            input: 5000000,
            cached: 100000,
            output: 8000000,
          },
        ],
      }),
    }),
  )
})
it('selects pool models and replaces prices when changing the selected model', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify(
            url.includes('/models')
              ? modelCatalog
              : {
                  prices: {
                    [new URL(url, 'http://example.test').searchParams.get(
                      'model',
                    )!]: {
                      input: url.includes('synthetic-pro') ? 5000000 : 1000000,
                      cached: 100000,
                      output: 8000000,
                      source: 'synthetic',
                    },
                  },
                },
          ),
        ),
    ),
  )
  const user = userEvent.setup()
  const submit = mountModelPrices()
  await user.type(screen.getByLabelText('Resource allowance name'), 'Synthetic')
  await openModelPrices(user)
  await user.click(
    screen.getByRole('menuitemradio', { name: 'synthetic-basic' }),
  )
  await waitFor(() =>
    expect(
      (screen.getByLabelText('Input · USD / M') as HTMLInputElement).value,
    ).toBe('1'),
  )
  await user.clear(screen.getByLabelText('Input · USD / M'))
  await user.type(screen.getByLabelText('Input · USD / M'), '2')
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'synthetic-pro' }))
  await waitFor(() =>
    expect(
      (screen.getByLabelText('Input · USD / M') as HTMLInputElement).value,
    ).toBe('5'),
  )
  await user.type(screen.getByLabelText('Allowance for synthetic-member'), '10')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: expect.objectContaining({
        rates: [
          {
            model: 'synthetic-pro',
            input: 5000000,
            cached: 100000,
            output: 8000000,
          },
        ],
      }),
    }),
  )
})
it('ignores a late price response after switching models and explains missing prices', async () => {
  let resolve!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/models'))
        return Promise.resolve(new Response(JSON.stringify(modelCatalog)))
      if (url.includes('synthetic-basic'))
        return new Promise<Response>((done) => {
          resolve = done
        })
      return Promise.resolve(new Response(JSON.stringify({ prices: {} })))
    }),
  )
  const user = userEvent.setup()
  mountModelPrices()
  await openModelPrices(user)
  await user.click(
    screen.getByRole('menuitemradio', { name: 'synthetic-basic' }),
  )
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'synthetic-pro' }))
  expect(
    await screen.findByText(
      'No catalog price for this model. Enter prices manually.',
    ),
  ).toBeTruthy()
  await user.type(screen.getByLabelText('Input · USD / M'), '3')
  await act(async () =>
    resolve(
      new Response(
        JSON.stringify({
          prices: {
            'synthetic-basic': {
              input: 1000000,
              cached: 100000,
              output: 8000000,
              source: 'synthetic',
            },
          },
        }),
      ),
    ),
  )
  expect(
    (screen.getByLabelText('Input · USD / M') as HTMLInputElement).value,
  ).toBe('3')
  expect(
    screen.getByRole('button', { name: 'Model ID' }).textContent,
  ).toContain('synthetic-pro')
})
it('reports a failed catalog and can retry before selecting a model', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response('{}', { status: 503 }))
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  mountModelPrices()
  await user.click(screen.getByLabelText('By amount'))
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  expect(
    await screen.findByText(
      'Could not retrieve the model catalog. Try again later.',
    ),
  ).toBeTruthy()
  expect(
    (
      screen.getByRole('button', {
        name: /Add all available models/,
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  fetch.mockResolvedValue(new Response(JSON.stringify(modelCatalog)))
  await user.click(screen.getByRole('button', { name: 'Refresh list' }))
  await waitFor(() =>
    expect(
      (screen.getByRole('button', { name: 'Model ID' }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  )
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  expect(
    screen.getByRole('menuitemradio', { name: 'synthetic-basic' }),
  ).toBeTruthy()
})

it('does not bulk add an incomplete pool catalog', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ ...modelCatalog, unknown_accounts: 1 })),
      ),
  )
  const user = userEvent.setup()
  mountModelPrices()
  await user.click(screen.getByLabelText('By amount'))
  expect(
    await screen.findByText(
      'Some account catalogs are unavailable. This list may be incomplete.',
    ),
  ).toBeTruthy()
  expect(
    (
      screen.getByRole('button', {
        name: 'Add all available models (2)',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
})

it('explains when the pool catalog exceeds the model price limit', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...modelCatalog,
          models: Array.from({ length: 129 }, (_, index) => ({
            id: `synthetic-${index}`,
            object: 'model',
            owned_by: 'synthetic',
          })),
        }),
      ),
    ),
  )
  const user = userEvent.setup()
  mountModelPrices()
  await user.click(screen.getByLabelText('By amount'))
  expect(
    await screen.findByText('Up to 128 model prices can be configured.'),
  ).toBeTruthy()
  expect(
    (
      screen.getByRole('button', {
        name: 'Add all available models (129)',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
})

it('retains a saved model and its custom prices when the catalog no longer lists it', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(modelCatalog)))
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  const submit = mountModelPrices('synthetic-retired')
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  expect(
    await screen.findByRole('menuitemradio', { name: 'synthetic-basic' }),
  ).toBeTruthy()
  expect(
    screen
      .getByRole('menuitemradio', { name: 'synthetic-retired' })
      .getAttribute('aria-checked'),
  ).toBe('true')
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', { name: 'Save resource allowance' }),
  )
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      config: expect.objectContaining({
        rates: [
          {
            model: 'synthetic-retired',
            input: 3000000,
            cached: 0,
            output: 7000000,
          },
        ],
      }),
    }),
  )
  expect(
    fetch.mock.calls.every(([url]) => String(url).endsWith('/groups/2/models')),
  ).toBe(true)
})
it('does not fill a replacement row from a removed row and recovers from a price failure', async () => {
  let resolve!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/models'))
        return Promise.resolve(new Response(JSON.stringify(modelCatalog)))
      if (url.includes('synthetic-basic'))
        return new Promise<Response>((done) => {
          resolve = done
        })
      return Promise.resolve(new Response('{}', { status: 503 }))
    }),
  )
  const user = userEvent.setup()
  mountModelPrices()
  await openModelPrices(user)
  await user.click(
    screen.getByRole('menuitemradio', { name: 'synthetic-basic' }),
  )
  await user.click(screen.getByRole('button', { name: 'Remove model price 1' }))
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  await user.click(screen.getByRole('button', { name: 'Model ID' }))
  await user.click(screen.getByRole('menuitemradio', { name: 'synthetic-pro' }))
  expect(
    await screen.findByText(
      'Could not load model prices. Enter prices manually.',
    ),
  ).toBeTruthy()
  await user.type(screen.getByLabelText('Input · USD / M'), '9')
  await act(async () =>
    resolve(
      new Response(
        JSON.stringify({
          prices: {
            'synthetic-basic': {
              input: 1000000,
              cached: 100000,
              output: 8000000,
              source: 'synthetic',
            },
          },
        }),
      ),
    ),
  )
  expect(
    (screen.getByLabelText('Input · USD / M') as HTMLInputElement).value,
  ).toBe('9')
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  await user.click(screen.getAllByRole('button', { name: 'Model ID' })[1])
  expect(
    screen.queryByRole('menuitemradio', { name: 'synthetic-pro' }),
  ).toBeNull()
  expect(
    screen.getByRole('menuitemradio', { name: 'synthetic-basic' }),
  ).toBeTruthy()
})
it('shows an empty pool catalog honestly and allows reloading it', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ ...modelCatalog, models: [] })),
      ),
  )
  const user = userEvent.setup()
  mountModelPrices()
  await user.click(screen.getByLabelText('By amount'))
  expect(
    await screen.findByText('No models are currently available to this pool.'),
  ).toBeTruthy()
  expect(
    (
      screen.getByRole('button', {
        name: 'Add all available models (0)',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Add model price' }))
  expect(
    await screen.findByText('No models are currently available to this pool.'),
  ).toBeTruthy()
  expect(
    (screen.getByRole('button', { name: 'Model ID' }) as HTMLButtonElement)
      .disabled,
  ).toBe(true)
  expect(screen.getByRole('button', { name: 'Refresh list' })).toBeTruthy()
})
