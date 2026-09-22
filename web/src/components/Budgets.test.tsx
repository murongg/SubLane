import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated } from '@/test/fixtures'

const member = {
  id: 2,
  username: 'member-test',
  role: 'member',
  enabled: true,
  created_at: 1900000000,
}
const budget = {
  id: 1,
  group_id: 0,
  group_name: '',
  model: '',
  period: 'day' as const,
  limit: 1000,
  enabled: true,
  created_at: 1900000000,
  used: 50,
  pending: 0,
  window_start: 1900000000,
  reset_at: 1900086400,
}
const response = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), { status })
function open() {
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/members'] }),
      )}
    />,
  )
}

it('creates a scoped budget and edits its limit without changing its scope', async () => {
  const rules: (typeof budget)[] = []
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/groups')
        return Promise.resolve(response({ groups: [] }))
      if (url.endsWith('/budgets')) {
        if (init?.method === 'PUT') {
          const input = JSON.parse(String(init.body))
          rules.splice(0, rules.length, { ...budget, ...input })
          return Promise.resolve(response(rules[0]))
        }
        return Promise.resolve(response({ rules, pending: [] }))
      }
      return Promise.resolve(response({ members: [member], next_cursor: 0 }))
    })
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  open()
  await user.click(
    await screen.findByRole('button', { name: 'More actions for member-test' }),
  )
  await user.click(screen.getByRole('menuitem', { name: 'Token budgets' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Token budgets for member-test',
  })
  await within(dialog).findByText(
    'No token budgets for keys outside allocation schemes.',
  )
  await user.click(within(dialog).getByRole('button', { name: 'Add budget' }))
  await user.type(within(dialog).getByLabelText('Model ID'), 'synthetic-model')
  await user.type(within(dialog).getByLabelText('Token limit (M)'), '1.25')
  await user.click(within(dialog).getByRole('button', { name: 'Save budget' }))
  await within(dialog).findByText('synthetic-model')
  const saved = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')
  expect(JSON.parse(String(saved?.[1]?.body))).toEqual({
    group_id: 0,
    model: 'synthetic-model',
    period: 'day',
    limit: 1250000,
    enabled: true,
  })
  await user.click(within(dialog).getByRole('button', { name: 'Edit budget' }))
  expect(
    (within(dialog).getByLabelText('Model ID') as HTMLInputElement).disabled,
  ).toBe(true)
  const limit = within(dialog).getByLabelText('Token limit (M)')
  await user.clear(limit)
  await user.type(limit, '2')
  await user.click(within(dialog).getByRole('button', { name: 'Save budget' }))
  await waitFor(() =>
    expect(
      fetch.mock.calls.filter(([, init]) => init?.method === 'PUT'),
    ).toHaveLength(2),
  )
  await within(dialog).findByText(/2 M tokens/)
})

it('shows load failures and allows an administrator to settle unknown usage', async () => {
  let failed = true,
    pending = true
  const fetch = vi.fn().mockImplementation((url: string) => {
    if (url === '/api/auth/state')
      return Promise.resolve(response(authenticated))
    if (url === '/api/groups') return Promise.resolve(response({ groups: [] }))
    if (url.endsWith('/budgets/settle')) {
      pending = false
      return Promise.resolve(new Response(null, { status: 204 }))
    }
    if (url.endsWith('/budgets'))
      return Promise.resolve(
        failed
          ? response({ error: 'unavailable' }, 503)
          : response({
              rules: [{ ...budget, pending: pending ? 1 : 0 }],
              pending: pending
                ? [
                    {
                      request_id: 'req_synthetic',
                      started_at: 1900000000,
                      known_tokens: 50,
                    },
                  ]
                : [],
            }),
      )
    return Promise.resolve(response({ members: [member], next_cursor: 0 }))
  })
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  open()
  await user.click(
    await screen.findByRole('button', { name: 'More actions for member-test' }),
  )
  await user.click(screen.getByRole('menuitem', { name: 'Token budgets' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Token budgets for member-test',
  })
  await within(dialog).findByText('Unable to load token budgets.')
  failed = false
  await user.click(within(dialog).getByRole('button', { name: 'Reconnect' }))
  await within(dialog).findByText('req_synthetic')
  const tokens = within(dialog).getByLabelText(
    'Total tokens (M) for req_synthetic',
  )
  await user.type(tokens, '0.00007')
  await user.click(within(dialog).getByRole('button', { name: 'Settle usage' }))
  await waitFor(() =>
    expect(within(dialog).queryByText('req_synthetic')).toBeNull(),
  )
  const settled = fetch.mock.calls.find(
    ([url, init]) => url.endsWith('/settle') && init?.method === 'POST',
  )
  expect(JSON.parse(String(settled?.[1]?.body))).toEqual({
    request_id: 'req_synthetic',
    tokens: 70,
  })
})

it('keeps the reset period visible when a budget is pending or exhausted', async () => {
  const { BudgetList } = await import('./BudgetList')
  render(
    <BudgetList
      rules={[
        { ...budget, pending: 1 },
        { ...budget, id: 2, period: 'month', used: 1000 },
      ]}
    />,
  )
  expect(screen.getByText('Daily · UTC')).toBeTruthy()
  expect(screen.getByText('Monthly · UTC')).toBeTruthy()
  expect(screen.getByText('Usage pending settlement')).toBeTruthy()
  expect(screen.getByText('Budget exhausted')).toBeTruthy()
})

it('preserves single-token precision in million-token inputs', async () => {
  const { parseBudgetMillions } = await import('@/lib/budgets')
  expect(parseBudgetMillions('0.000001')).toBe(1)
  expect(parseBudgetMillions('1.000001')).toBe(1_000_001)
  expect(parseBudgetMillions('1000000')).toBe(1_000_000_000_000)
  expect(parseBudgetMillions('0.0000001')).toBeNull()
  expect(parseBudgetMillions('')).toBeNull()
  expect(parseBudgetMillions('-1')).toBeNull()
})
