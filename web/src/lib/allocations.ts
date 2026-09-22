import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { request, ApiError } from './request'
import { authKey, type AuthState } from './auth'

const integer = z.number().int().nonnegative()
const memberSchema = z.object({
  id: integer,
  username: z.string(),
  enabled: z.boolean(),
})
export const teamSchema = z.object({
  id: integer,
  name: z.string(),
  enabled: z.boolean(),
  member_ids: z.array(integer).max(100),
  group_ids: z.array(integer).max(32).optional(),
  members: z.array(memberSchema).max(100),
  created_at: integer,
})
const modeSchema = z.enum(['ratio', 'amount', 'tokens'])
const rateSchema = z.object({
  model: z.string(),
  input: integer,
  cached: integer,
  output: integer,
})
const priceSchema = z.object({
  input: integer,
  cached: integer,
  output: integer,
  source: z.string(),
})
const configSchema = z.object({
  mode: modeSchema,
  period: z.enum(['upstream', 'day', 'month']),
  members: z.array(z.object({ user_id: integer, limit: integer })).max(100),
  rates: z.array(rateSchema).max(128),
  allow_idle_borrow: z.boolean().optional(),
})
const revisionSchema = z.object({ effective_at: integer, config: configSchema })
export const schemeSchema = revisionSchema.extend({
  id: integer,
  name: z.string(),
  team_id: integer,
  team_name: z.string(),
  group_id: integer,
  group_name: z.string(),
  enabled: z.boolean(),
  created_at: integer,
  next: revisionSchema.nullable(),
})
const balanceSchema = z.object({
  user_id: integer,
  username: z.string(),
  mode: modeSchema,
  limit: integer,
  used: integer,
  borrowed: integer.optional(),
  tokens: integer,
  pending: integer,
  reset_at: integer,
  window_id: integer,
  window_kind: z.string(),
  account_id: z.string().optional(),
  account_label: z.string().optional(),
  syncing: integer.optional(),
  sync_paused: z.boolean().optional(),
})
const pendingSchema = z.object({
  request_id: z.string(),
  user_id: integer,
  mode: modeSchema,
  model: z.string(),
  state: z.string(),
  automatic: z.boolean().optional(),
  input: integer,
  output: integer,
  cached: integer,
  debits: z.array(
    z.object({
      window_id: integer,
      kind: z.string(),
      reset_at: integer,
      points: integer,
      reconciled: z.boolean(),
    }),
  ),
})
export const allocationDetailSchema = schemeSchema.extend({
  balances: z.array(balanceSchema),
  pending: z.array(pendingSchema).max(256),
  unassigned: integer,
  available: z.boolean(),
})
export type Team = z.infer<typeof teamSchema>
export type Scheme = z.infer<typeof schemeSchema>
export type AllocationDetail = z.infer<typeof allocationDetailSchema>
export type AllocationPending = z.infer<typeof pendingSchema>
export type AllocationMode = z.infer<typeof modeSchema>
export type AllocationConfig = z.infer<typeof configSchema>
export type SchemeInput = {
  name: string
  team_id: number
  group_id: number
  enabled: boolean
  start_next: boolean
  config: AllocationConfig
}
export const teamsOptions = queryOptions({
  queryKey: ['teams'],
  queryFn: ({ signal }) =>
    request('/api/teams', z.object({ teams: z.array(teamSchema).max(64) }), {
      signal,
    }),
})
export const schemesOptions = queryOptions({
  queryKey: ['allocations'],
  refetchInterval: 30_000,
  queryFn: ({ signal }) =>
    request(
      '/api/allocations',
      z.object({ schemes: z.array(schemeSchema).max(32) }),
      { signal },
    ),
})
export function allocationOptions(id: number) {
  return queryOptions({
    queryKey: ['allocation', id],
    queryFn: ({ signal }) =>
      request(`/api/allocations/${id}`, allocationDetailSchema, { signal }),
    refetchInterval: 30_000,
  })
}
export function ownAllocationOptions(client: QueryClient, user: number) {
  return queryOptions({
    queryKey: ['own-allocations', user],
    enabled: () => client.getQueryData<AuthState>(authKey)?.user?.id === user,
    queryFn: ({ signal }) =>
      request(
        '/api/me/allocations',
        z.object({ schemes: z.array(allocationDetailSchema).max(32) }),
        { signal },
      ),
    refetchInterval: 30_000,
  })
}
export function saveTeam({
  id,
  ...input
}: {
  id?: number
  name: string
  enabled: boolean
  member_ids: number[]
  group_ids?: number[]
}) {
  return request(id ? `/api/teams/${id}` : '/api/teams', teamSchema, {
    method: id ? 'PATCH' : 'POST',
    body: JSON.stringify(input),
  })
}
export function saveScheme({ id, ...input }: SchemeInput & { id?: number }) {
  return request(
    id ? `/api/allocations/${id}` : '/api/allocations',
    schemeSchema,
    { method: id ? 'PATCH' : 'POST', body: JSON.stringify(input) },
  )
}
export function lookupModelPrice(model: string) {
  return request(
    `/api/allocations/prices?model=${encodeURIComponent(model)}`,
    z.object({ prices: z.record(z.string(), priceSchema) }),
  ).then((result) => result.prices[model] ?? Object.values(result.prices)[0])
}
export function refreshAllocation(id: number) {
  return request(`/api/allocations/${id}/refresh`, allocationDetailSchema, {
    method: 'POST',
    body: '{}',
  })
}
export function reserveAllocation(id: number, points: number) {
  return request(`/api/allocations/${id}/reserve`, z.undefined(), {
    method: 'POST',
    body: JSON.stringify({ points }),
  })
}
export function settleAllocation(
  id: number,
  input: {
    request_id: string
    input: number
    output: number
    cached: number
    points: Record<number, number>
  },
) {
  return request(`/api/allocations/${id}/settle`, z.undefined(), {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export const modeLabels = {
  ratio: 'allocationRatio',
  amount: 'allocationAmount',
  tokens: 'allocationTokens',
} as const
export function parseAllocationValue(
  value: string,
  mode: AllocationMode,
): number | null {
  const precision = mode === 'ratio' ? 2 : 6
  if (
    !new RegExp(
      `^(?:\\d+(?:\\.\\d{1,${precision}})?|\\.\\d{1,${precision}})$`,
    ).test(value)
  )
    return null
  const [whole, fraction = ''] = value.split('.')
  const n =
    Number(whole || '0') * 10 ** precision +
    Number(fraction.padEnd(precision, '0'))
  return Number.isSafeInteger(n) && n <= 1_000_000_000_000 ? n : null
}
export function allocationValue(n: number, mode: AllocationMode) {
  return String(n / (mode === 'ratio' ? 100 : 1_000_000))
}
export function allocationErrorKey(error: Error) {
  if (error instanceof ApiError) {
    if (
      error.code === 'allocation_pool_conflict' ||
      error.code === 'allocation_pool_locked'
    )
      return 'allocationPoolConflict'
    if (error.code === 'allocation_snapshot_required')
      return 'allocationSnapshotRequired'
    if (error.code === 'allocation_syncing') return 'allocationSyncPaused'
    if (error.code === 'invalid_allocation_input') return 'allocationInvalid'
    if (error.code === 'invalid_allocation_settlement')
      return 'allocationSettlementConflict'
    if (error.code === 'allocation_pending') return 'allocationPending'
  }
  return 'allocationFailed'
}

export function setSchemeEnabled(id: number, enabled: boolean) {
  return request(`/api/allocations/${id}/enabled`, z.undefined(), {
    method: 'POST',
    body: JSON.stringify({ enabled }),
  })
}

export function availableAllocationPools<
  T extends { id: number; enabled: boolean; account_count: number },
>(pools: T[], schemes: { group_id: number }[]): T[] {
  return pools.filter(
    (pool) =>
      pool.id !== 1 &&
      pool.enabled &&
      pool.account_count > 0 &&
      !schemes.some((scheme) => scheme.group_id === pool.id),
  )
}
