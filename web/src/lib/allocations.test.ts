import { afterEach, expect, it, vi } from 'vitest'
import { z } from 'zod'
import {
  allocationDetailSchema,
  schemeSchema,
  previewInitial,
} from './allocations'

afterEach(() => vi.unstubAllGlobals())

it('reads a server creation preview without creating an allowance', async () => {
  const fetch = vi.fn(async (url: string) => {
    expect(url).toBe('/api/allocations/preview')
    return new Response(
      JSON.stringify({
        write_conditions: {
          expected_current_revision_id: null,
          expected_next_revision_id: null,
        },
        proposed_effective_at: 1900086400,
        proposed_accounting_anchor_at: 1900086400,
      }),
      { status: 200 },
    )
  })
  vi.stubGlobal('fetch', fetch)
  const preview = await previewInitial({
    name: 'Synthetic preview',
    group_id: 2,
    enabled: true,
    start_next: true,
    config: {
      mode: 'tokens',
      period: 'day',
      members: [{ user_id: 101, limit: 100 }],
      rates: [],
    },
  })
  expect(preview.proposed_effective_at).toBe(1900086400)
  expect(fetch.mock.calls[0]?.[0]).toBe('/api/allocations/preview')
  expect(fetch).toHaveBeenCalledOnce()
})

it('keeps legacy and duration-ratio schemes readable in the same response', () => {
  const config = {
    mode: 'ratio',
    period: 'month',
    ratio_unit: 'tokens',
    total: 1_000_000,
    members: [{ user_id: 101, limit: 10_000 }],
    rates: [],
  }
  const legacy = {
    id: 1,
    name: 'Synthetic legacy',
    group_id: 2,
    group_name: 'Synthetic group',
    enabled: true,
    created_at: 1_900_000_000,
    effective_at: 1_900_000_000,
    config,
    next: null,
  }
  const duration = {
    ...legacy,
    id: 2,
    name: 'Synthetic duration shares',
    config: {
      ...config,
      total: undefined,
      period: 'durations',
      windows: [
        { duration_seconds: 18_000, limit: 1_000_000 },
        { duration_seconds: 604_800, limit: 5_000_000 },
      ],
    },
    accounting_anchor_at: 1_900_000_000,
    write_conditions: {
      expected_current_revision_id: 41,
      expected_next_revision_id: null,
    },
  }
  expect(
    z.object({ schemes: z.array(schemeSchema) }).parse({
      schemes: [legacy, duration],
    }).schemes,
  ).toHaveLength(2)
  expect(
    allocationDetailSchema.parse({
      ...duration,
      available: true,
      balances: [],
      pending: [],
    }).config.windows,
  ).toHaveLength(2)
  expect(schemeSchema.parse(duration)).toHaveProperty(
    'write_conditions.expected_current_revision_id',
    41,
  )
})
