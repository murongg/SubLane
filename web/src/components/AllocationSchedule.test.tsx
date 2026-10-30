import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { AllocationSchedule } from './AllocationSchedule'
import { schemeSchema } from '@/lib/allocations'

afterEach(() => vi.unstubAllGlobals())

it('keeps initial cancellation disabled and confirms the server successor before rescheduling', async () => {
  const config = {
    mode: 'tokens',
    period: 'day',
    members: [{ user_id: 101, limit: 100 }],
    rates: [],
  }
  const scheme = schemeSchema.parse({
    id: 1,
    name: 'Synthetic initial',
    group_id: 2,
    group_name: 'Synthetic group',
    enabled: true,
    created_at: 1900000000,
    effective_at: 1900086400,
    config,
    next: {
      effective_at: 1900086400,
      accounting_anchor_at: 1900086400,
      config,
    },
    write_conditions: {
      expected_current_revision_id: null,
      expected_next_revision_id: 41,
    },
  })
  const fetch = vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/preview'))
      return new Response(
        JSON.stringify({
          write_conditions: scheme.write_conditions,
          proposed_effective_at: 1900172800,
          proposed_accounting_anchor_at: 1900172800,
        }),
        { status: 200 },
      )
    expect(JSON.parse(options?.body as string)).toEqual({
      ...scheme.write_conditions,
      expected_effective_at: 1900172800,
    })
    return new Response(JSON.stringify(scheme), { status: 200 })
  })
  vi.stubGlobal('fetch', fetch)
  const changed = vi.fn(async () => {})
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AllocationSchedule scheme={scheme} onChanged={changed} />
    </QueryClientProvider>,
  )
  expect(
    (
      screen.getByRole('button', {
        name: 'Cancel scheduled change',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  await user.click(
    screen.getByRole('button', { name: 'Move to next boundary' }),
  )
  await user.click(
    await screen.findByRole('button', { name: 'Confirm start time' }),
  )
  await waitFor(() => expect(changed).toHaveBeenCalledOnce())
  expect(fetch).toHaveBeenCalledTimes(2)
})
