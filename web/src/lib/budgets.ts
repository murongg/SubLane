import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { ApiError, request } from './request'

export const budgetSchema = z.object({
  id: z.number().int().positive(),
  group_id: z.number().int().nonnegative(),
  group_name: z.string(),
  model: z.string().max(128),
  period: z.enum(['day', 'month']),
  limit: z.number().int().min(1).max(1_000_000_000_000),
  enabled: z.boolean(),
  created_at: z.number().int(),
  used: z.number().int().nonnegative(),
  pending: z.number().int().nonnegative(),
  window_start: z.number().int(),
  reset_at: z.number().int(),
})
const pendingSchema = z.object({
  request_id: z.string(),
  started_at: z.number().int(),
  known_tokens: z.number().int().nonnegative(),
})
const pageSchema = z.object({
  rules: z.array(budgetSchema).max(64),
  pending: z.array(pendingSchema).max(256),
})
export type Budget = z.infer<typeof budgetSchema>
export type BudgetPending = z.infer<typeof pendingSchema>
export type BudgetInput = Pick<
  Budget,
  'group_id' | 'model' | 'period' | 'limit' | 'enabled'
>
export function budgetOptions(userID: number) {
  return queryOptions({
    queryKey: ['member-budgets', userID],
    queryFn: ({ signal }) =>
      request(`/api/members/${userID}/budgets`, pageSchema, { signal }),
  })
}
export function ownBudgetOptions(client: QueryClient, userID: number) {
  return queryOptions({
    queryKey: ['own-budgets', userID],
    enabled: () => client.getQueryData<AuthState>(authKey)?.user?.id === userID,
    queryFn: ({ signal }) => request('/api/me/budgets', pageSchema, { signal }),
    refetchInterval: 30_000,
  })
}
export function saveBudget({
  userID,
  input,
}: {
  userID: number
  input: BudgetInput
}) {
  return request(`/api/members/${userID}/budgets`, budgetSchema, {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}
export function settleBudget({
  userID,
  request_id,
  tokens,
}: {
  userID: number
  request_id: string
  tokens: number
}) {
  return request(`/api/members/${userID}/budgets/settle`, z.undefined(), {
    method: 'POST',
    body: JSON.stringify({ request_id, tokens }),
  })
}
export function budgetErrorKey(error: Error) {
  if (error instanceof ApiError) {
    if (error.code === 'token_budget_rule_limit') return 'budgetRuleLimit'
    if (error.code === 'invalid_token_settlement')
      return 'budgetSettlementConflict'
    if (error.code === 'token_accounting_unavailable')
      return 'budgetAccountingFailed'
  }
  return 'budgetSaveFailed'
}

// Parse decimal millions as integer parts: floating-point rounding must not create or lose tokens.
export function parseBudgetMillions(value: string): number | null {
  if (!/^(?:\d+(?:\.\d{1,6})?|\.\d{1,6})$/.test(value)) return null
  const [whole, fraction = ''] = value.split('.')
  const tokens =
    Number(whole || '0') * 1_000_000 + Number(fraction.padEnd(6, '0'))
  return Number.isSafeInteger(tokens) ? tokens : null
}
