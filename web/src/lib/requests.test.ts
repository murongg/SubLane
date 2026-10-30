import { QueryClient } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { authKey } from './auth'
import { cacheHitRate, requestExport } from './requests'
import { memberAuthenticated } from '@/test/fixtures'

function record(id: number) {
  return {
    id,
    user_id: 2,
    key_id: 1,
    group_id: 1,
    account_id: '',
    account_name: '',
    provider: 'codex',
    model: 'synthetic-model',
    transport: 'http',
    operation: 'responses',
    started_at: 1900000000,
    duration_ms: 50,
    request_id: `req_synthetic_${id}`,
    first_token_ms: null,
    outcome: 'success',
    error_code: '',
    upstream_status: 200,
    input_tokens: null,
    output_tokens: null,
    cached_tokens: null,
    username: 'synthetic-member',
    key_name: 'Synthetic key',
    group_name: 'Synthetic group',
  }
}

it('exports every cursor page with the same filters and personal scope', async () => {
  const client = new QueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  const fetch = vi.fn(async (url: string) => {
    const query = new URL(url, 'http://example.test')
    const cursor = Number(query.searchParams.get('cursor'))
    expect(query.pathname).toBe('/api/me/requests')
    expect(query.searchParams.get('account_id')).toBeNull()
    expect(query.searchParams.get('user_id')).toBeNull()
    expect(query.searchParams.get('model')).toBe('synthetic-model')
    return new Response(
      JSON.stringify(
        cursor
          ? { requests: [record(100), record(99)], next_cursor: 0 }
          : {
              requests: Array.from({ length: 100 }, (_, index) =>
                record(200 - index),
              ),
              next_cursor: 101,
            },
      ),
    )
  })
  vi.stubGlobal('fetch', fetch)
  const rows = await requestExport(
    client,
    memberAuthenticated.user.id,
    'personal',
    { model: 'synthetic-model', account_id: 'hidden', user_id: 99 },
    new AbortController().signal,
  )
  expect(rows).toHaveLength(102)
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('stops exporting when the authenticated identity changes', async () => {
  const client = new QueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      client.setQueryData(authKey, { user: null })
      return new Response(
        JSON.stringify({ requests: [record(1)], next_cursor: 0 }),
      )
    }),
  )
  await expect(
    requestExport(
      client,
      memberAuthenticated.user.id,
      'personal',
      {},
      new AbortController().signal,
    ),
  ).rejects.toThrow('unauthorized')
})

it.each([
  [1250, 1070, 0.856],
  [1000, 0, 0],
  [1000, 1000, 1],
  [0, 0, null],
  [null, 100, null],
  [1000, null, null],
  [100, 101, null],
  [-1, 0, null],
  [100, -1, null],
])(
  'calculates cache hit rate for input=%s, cached=%s',
  (input, cached, expected) => {
    expect(cacheHitRate(input, cached)).toBe(expected)
  },
)

it('validates and retains fixed routing codes and aggregate counts', async () => {
  const client = new QueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  const route = {
    version: 1,
    strategy: 'quota_weighted',
    result: 'selected',
    candidates: 2,
    parent_fallback: false,
    exclusions: {
      model: 1,
      policy: 0,
      quota: 0,
      cooldown: 0,
      concurrency: 0,
      unavailable: 0,
    },
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            requests: [{ ...record(1), route }],
            next_cursor: 0,
          }),
        ),
    ),
  )
  const records = await requestExport(
    client,
    memberAuthenticated.user.id,
    'personal',
    {},
    new AbortController().signal,
  )
  expect(records[0]).toHaveProperty('route', route)
})

it.each([
  { strategy: 'synthetic-private-strategy' },
  { candidates: 10001 },
  { account_id: 'synthetic-private-account' },
  {
    exclusions: {
      model: -1,
      policy: 0,
      quota: 0,
      cooldown: 0,
      concurrency: 0,
      unavailable: 0,
    },
  },
])(
  'rejects incompatible routing metadata at the API boundary: %j',
  async (change) => {
    const client = new QueryClient()
    client.setQueryData(authKey, memberAuthenticated)
    const route = {
      version: 1,
      strategy: 'rotation',
      result: 'selected',
      candidates: 1,
      parent_fallback: false,
      exclusions: {
        model: 0,
        policy: 0,
        quota: 0,
        cooldown: 0,
        concurrency: 0,
        unavailable: 0,
      },
      ...change,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              requests: [{ ...record(1), route }],
              next_cursor: 0,
            }),
          ),
      ),
    )
    await expect(
      requestExport(
        client,
        memberAuthenticated.user.id,
        'personal',
        {},
        new AbortController().signal,
      ),
    ).rejects.toThrow('invalid_response')
  },
)
